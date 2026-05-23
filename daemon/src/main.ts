#!/usr/bin/env bun
// hike-fleetd — long-lived background daemon for `hike fleet`.
// Spawned by the Go CLI via internal/fleet/daemon.go.
//
// CLI args (all required unless noted):
//   --root <dir>             hike root (contains hike.yaml)
//   --pid-file <path>        where to write our PID
//   --port-file <path>       where to write our MCP HTTP port
//   --ctl-sock <path>        unix socket path for JSON-RPC ctl
//   --state-db <path>        SQLite path
//   --worker-log-dir <path>  dir for per-worker .jsonl logs
//   --manager-prompt <path>  manager prompt file (written if missing)
//   --worker-prompt <path>   worker prompt file (written if missing)
//   --projects-md <path>     PROJECTS.md path
//   --port <int>             optional MCP HTTP port (0 = pick free)
import { existsSync, writeFileSync, readFileSync, mkdirSync, unlinkSync } from "node:fs";
import { dirname } from "node:path";
import { State } from "./state/db.ts";
import { startCtlServer } from "./ctl/server.ts";
import { startMcpHttpServer } from "./mcp/server.ts";
import { Supervisor } from "./supervisor/registry.ts";
import { startProjectsWatcher } from "./projects/watcher.ts";
import { log } from "./log.ts";
import managerPromptTemplate from "./prompts/manager.md" with { type: "text" };
import workerPromptTemplate from "./prompts/worker.md" with { type: "text" };

type Args = {
  root: string;
  pidFile: string;
  portFile: string;
  ctlSock: string;
  stateDb: string;
  workerLogDir: string;
  managerPrompt: string;
  workerPrompt: string;
  projectsMd: string;
  port: number;
};

function parseArgs(argv: string[]): Args {
  const map = new Map<string, string>();
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a && a.startsWith("--")) {
      const key = a.slice(2);
      const val = argv[i + 1] ?? "";
      map.set(key, val);
      i++;
    }
  }
  const get = (k: string, optional = false): string => {
    const v = map.get(k);
    if (v === undefined || v === "") {
      if (optional) return "";
      throw new Error(`missing required arg --${k}`);
    }
    return v;
  };
  return {
    root: get("root"),
    pidFile: get("pid-file"),
    portFile: get("port-file"),
    ctlSock: get("ctl-sock"),
    stateDb: get("state-db"),
    workerLogDir: get("worker-log-dir"),
    managerPrompt: get("manager-prompt"),
    workerPrompt: get("worker-prompt"),
    projectsMd: get("projects-md"),
    port: parseInt(get("port", true) || "0", 10),
  };
}

async function main() {
  const args = parseArgs(process.argv.slice(2));

  log.info("hike-fleetd starting", { pid: process.pid, root: args.root });

  // Ensure dirs exist.
  for (const f of [args.pidFile, args.portFile, args.ctlSock, args.stateDb, args.workerLogDir]) {
    mkdirSync(dirname(f), { recursive: true });
  }
  mkdirSync(args.workerLogDir, { recursive: true });

  // Write prompt files if absent — they're user-editable.
  if (!existsSync(args.managerPrompt)) {
    writeFileSync(args.managerPrompt, managerPromptTemplate);
  }
  if (!existsSync(args.workerPrompt)) {
    writeFileSync(args.workerPrompt, workerPromptTemplate);
  }

  // Open SQLite + run migrations.
  const state = new State(args.stateDb);

  // Reconcile any workers stuck in transitional states from a crash.
  const stuck = state.liveOrTransitionalWorkers();
  for (const w of stuck) {
    log.warn("found stuck worker on startup; marking crashed", { id: w.id, state: w.state });
    state.updateWorker(w.id, { state: "crashed", endedAt: Date.now() });
    state.setProjectState(w.projectName, "idle", null);
  }

  // Resolve the path to the hike binary we'll shell out to.
  const hikeBinary =
    process.env["HIKE_BIN"] ||
    (await locateHikeBinary()) ||
    "hike";

  // Build the service bundle handed to MCP tool factories.
  const supervisor = new Supervisor(/* maxConcurrent v0 */ 1);
  const startedAt = Date.now();

  // Services is a single mutable object; tool factories close over this
  // reference, so updating fields after the MCP server starts propagates
  // to all tool handlers. (Fixes the worker MCP URL chicken-and-egg.)
  const services = {
    state,
    supervisor,
    root: args.root,
    projectsMD: args.projectsMd,
    workerPromptFile: args.workerPrompt,
    workerMcpUrl: "", // patched after MCP starts; tools read svc.workerMcpUrl at call time
    workerConfig: {
      model: process.env["HIKE_WORKER_MODEL"] || "claude-sonnet-4-5-20250929",
      permissionMode: "acceptEdits" as const,
      allowedTools: [
        "Read",
        "Write",
        "Edit",
        "Grep",
        "Glob",
        "Bash(git diff:*, git status:*, git log:*, npm:*, pnpm:*, go:*, cargo:*)",
        "mcp__hike-fleet__fleet_post_status",
        "mcp__hike-fleet__fleet_post_finding",
        "mcp__hike-fleet__fleet_check_inbox",
        "mcp__hike-fleet__fleet_send_to_manager",
      ],
      disallowedTools: ["Bash(rm:*)", "Bash(git push:*)", "Bash(gh pr*)"],
    },
    workerLogDir: args.workerLogDir,
    hikeBinary,
  };

  const mcp = await startMcpHttpServer(services, args.port);

  // Now that MCP HTTP is up, patch the worker URL into the services object.
  services.workerMcpUrl = mcp.url.worker;

  // Now write PID + port files so the parent (Go) can detect "ready".
  writeFileSync(args.pidFile, String(process.pid));
  writeFileSync(args.portFile, String(mcp.port));

  // ctl.sock server for the Go CLI.
  let isShuttingDown = false;
  const ctl = startCtlServer(args.ctlSock, {
    state,
    supervisor,
    pid: process.pid,
    mcpUrl: mcp.url.manager,
    startedAt,
    requestShutdown: async (graceMs, kill) => {
      if (isShuttingDown) return;
      isShuttingDown = true;
      await shutdown(graceMs, kill);
    },
  });

  // PROJECTS.md watcher.
  const watcher = startProjectsWatcher(args.projectsMd, state);

  // Signal handlers.
  const handleSignal = (sig: NodeJS.Signals) => {
    log.info("signal received", { sig });
    if (!isShuttingDown) {
      isShuttingDown = true;
      shutdown(30_000, sig === "SIGKILL").catch((err) => {
        log.error("shutdown error", { err: String(err) });
        process.exit(1);
      });
    }
  };
  process.on("SIGINT", handleSignal);
  process.on("SIGTERM", handleSignal);

  log.info("hike-fleetd ready", {
    pid: process.pid,
    mcp: mcp.url.manager,
    ctl: args.ctlSock,
    root: args.root,
  });

  async function shutdown(graceMs: number, _kill: boolean): Promise<void> {
    log.info("shutting down", { graceMs });
    await supervisor.stopAll(graceMs);
    await watcher.close().catch(() => {});
    ctl.close();
    await mcp.stop();
    state.close();
    try {
      unlinkSync(args.pidFile);
    } catch {
      /* ignore */
    }
    try {
      unlinkSync(args.portFile);
    } catch {
      /* ignore */
    }
    log.info("shutdown complete");
    // Give logs a tick to flush.
    setTimeout(() => process.exit(0), 50);
  }
}

async function locateHikeBinary(): Promise<string | null> {
  // Look for `hike` on PATH via Bun's $.
  try {
    const { $ } = await import("bun");
    const result = await $`which hike`.quiet();
    const path = result.text().trim();
    return path || null;
  } catch {
    return null;
  }
}

main().catch((err) => {
  log.error("fatal", { err: String(err), stack: err instanceof Error ? err.stack : undefined });
  // Use the parent fd directly to ensure the log lands before exit.
  process.stderr.write(`fatal: ${String(err)}\n`);
  process.exit(1);
});

// Keep TS happy if Bun strips unused imports.
void readFileSync;
