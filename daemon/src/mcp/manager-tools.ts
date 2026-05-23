// Manager-facing MCP tools. The manager Claude calls these to learn about
// fleet state, dispatch workers, etc.
import { z } from "zod";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { readFileSync, existsSync, appendFileSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import type { FleetServices } from "./services.ts";
import { spawnWorker, newWorkerId } from "../supervisor/spawn.ts";
import { log } from "../log.ts";

export function registerManagerTools(server: McpServer, svc: FleetServices) {
  server.registerTool(
    "fleet_list_projects",
    {
      description: "List all fleet-managed projects in this hike root with their current state.",
      inputSchema: {},
    },
    async () => {
      const rows = svc.state.listProjects();
      const summary = rows.length === 0
        ? "No projects yet. Use fleet_add_project to provision one."
        : rows
            .map(
              (p) =>
                `- ${p.name} (group=${p.groupName}, state=${p.state}${p.currentRun ? ", run=" + p.currentRun : ""}) — ${p.dir}`,
            )
            .join("\n");
      return { content: [{ type: "text", text: summary }] };
    },
  );

  server.registerTool(
    "fleet_project_info",
    {
      description: "Detail for one project: state, last status, recent runs.",
      inputSchema: {
        name: z.string().describe("Full project name (e.g. platform-bold-cedar)"),
      },
    },
    async (args) => {
      const p = svc.state.getProject(args.name);
      if (!p) {
        return { content: [{ type: "text", text: `Project '${args.name}' not found.` }], isError: true };
      }
      const latest = svc.state.latestStatusReport(args.name);
      const workers = svc.state.listWorkers({ project: args.name }).slice(0, 5);
      const planExists = existsSync(p.planPath);
      const lines = [
        `Project: ${p.name}`,
        `  Group:   ${p.groupName}`,
        `  Dir:     ${p.dir}`,
        `  State:   ${p.state}`,
        `  Run:     ${p.currentRun ?? "(none)"}`,
        `  Plan:    ${planExists ? p.planPath : "(no PLAN.md yet)"}`,
        `  Latest status: ${latest?.summary ?? "(none)"}`,
        `  Recent workers:`,
        ...(workers.length === 0
          ? ["    (none)"]
          : workers.map(
              (w) =>
                `    ${w.id} ${w.state} model=${w.model} started=${new Date(w.startedAt).toISOString()}`,
            )),
      ];
      return { content: [{ type: "text", text: lines.join("\n") }] };
    },
  );

  server.registerTool(
    "fleet_add_project",
    {
      description:
        "Provision a new hike project (worktrees + metadata) and mark it as fleet-managed. " +
        "Calls `hike create` under the hood. Appends an entry to PROJECTS.md.",
      inputSchema: {
        group: z.string().optional().describe("Group name; uses default group if omitted."),
        name: z.string().optional().describe("Project name suffix; random if omitted."),
        color: z.string().optional().describe("VS Code title bar color (red, blue, etc.)."),
      },
    },
    async (args) => {
      const hikeArgs = ["create"];
      if (args.group) hikeArgs.push("--group", args.group);
      if (args.name) hikeArgs.push("--name", args.name);
      if (args.color) hikeArgs.push("--color", args.color);
      log.info("fleet_add_project shelling out", { argv: [svc.hikeBinary, ...hikeArgs] });
      const result = spawnSync(svc.hikeBinary, hikeArgs, { cwd: svc.root, encoding: "utf-8" });
      if (result.status !== 0) {
        return {
          content: [{ type: "text", text: `hike create failed:\n${result.stderr || result.stdout}` }],
          isError: true,
        };
      }
      // Parse out the project name from stdout (best effort: scan for "platform-foo").
      // Easier: just re-scan project dirs and find ones not yet in the DB.
      const newProject = await reconcileNewProjects(svc);
      if (newProject) {
        appendProjectToProjectsMD(svc.projectsMD, newProject.name, newProject.groupName);
        return {
          content: [
            {
              type: "text",
              text: `Project ${newProject.name} created in group ${newProject.groupName}.\n${result.stdout}\nPLAN.md path: ${newProject.planPath}`,
            },
          ],
        };
      }
      return { content: [{ type: "text", text: result.stdout || "(no output)" }] };
    },
  );

  server.registerTool(
    "fleet_dispatch_worker",
    {
      description:
        "Dispatch a background Claude Code worker on the named project. " +
        "Reads PLAN.md from the project dir by default; pass `prompt` to override. " +
        "Returns immediately with the worker ID; the worker runs to completion in the background.",
      inputSchema: {
        project: z.string().describe("Full project name."),
        prompt: z.string().optional().describe("Override prompt instead of reading PLAN.md."),
        model: z.string().optional(),
        maxTurns: z.number().int().positive().optional(),
      },
    },
    async (args) => {
      if (!svc.supervisor.canSpawn()) {
        return {
          content: [
            {
              type: "text",
              text: `At max concurrent workers (${svc.supervisor.maxConcurrent}). Wait for one to finish or call fleet_stop_worker.`,
            },
          ],
          isError: true,
        };
      }
      const p = svc.state.getProject(args.project);
      if (!p) {
        return { content: [{ type: "text", text: `Project '${args.project}' not found.` }], isError: true };
      }
      let prompt = args.prompt;
      if (!prompt) {
        if (!existsSync(p.planPath)) {
          return {
            content: [
              {
                type: "text",
                text: `No PLAN.md at ${p.planPath}. Write one first, or pass an explicit prompt.`,
              },
            ],
            isError: true,
          };
        }
        prompt = readFileSync(p.planPath, "utf-8");
      }

      const workerId = newWorkerId();
      const workerLogPath = join(svc.workerLogDir, `${workerId}.jsonl`);
      const handle = spawnWorker(
        {
          id: workerId,
          projectName: p.name,
          projectDir: p.dir,
          prompt,
          model: args.model ?? svc.workerConfig.model,
          permissionMode: svc.workerConfig.permissionMode,
          allowedTools: svc.workerConfig.allowedTools,
          disallowedTools: svc.workerConfig.disallowedTools,
          workerMcpUrl: svc.workerMcpUrl,
          maxTurns: args.maxTurns ?? svc.workerConfig.maxTurns,
          workerLogPath,
          appendSystemPrompt: existsSync(svc.workerPromptFile)
            ? readFileSync(svc.workerPromptFile, "utf-8")
            : undefined,
        },
        {
          state: svc.state,
          onComplete: (h, outcome) => {
            svc.supervisor.unregister(h.id);
            log.info("worker complete", { id: h.id, outcome });
          },
        },
      );
      svc.supervisor.register(handle);

      return {
        content: [
          {
            type: "text",
            text: `Worker ${handle.id} dispatched on ${p.name}. Tail with: hike fleet logs ${p.name} ${handle.id} -f`,
          },
        ],
      };
    },
  );

  server.registerTool(
    "fleet_worker_status",
    {
      description: "Current state + last status report for a specific worker.",
      inputSchema: {
        workerId: z.string(),
      },
    },
    async (args) => {
      const w = svc.state.getWorker(args.workerId);
      if (!w) {
        return { content: [{ type: "text", text: `Worker '${args.workerId}' not found.` }], isError: true };
      }
      const latest = svc.state.latestStatusReport(w.projectName);
      const lines = [
        `Worker:  ${w.id}`,
        `  Project: ${w.projectName}`,
        `  State:   ${w.state}`,
        `  Model:   ${w.model ?? "(default)"}`,
        `  Started: ${new Date(w.startedAt).toISOString()}`,
        `  Ended:   ${w.endedAt ? new Date(w.endedAt).toISOString() : "(still running)"}`,
        `  Last heartbeat: ${w.lastHeartbeat ? new Date(w.lastHeartbeat).toISOString() : "(none)"}`,
        `  Session: ${w.sessionId ?? "(uninitialized)"}`,
        `  Latest status: ${latest?.summary ?? "(none)"}`,
      ];
      return { content: [{ type: "text", text: lines.join("\n") }] };
    },
  );

  server.registerTool(
    "fleet_worker_logs",
    {
      description: "Return the last N events from a worker's stream-json log.",
      inputSchema: {
        workerId: z.string(),
        lines: z.number().int().positive().default(50).optional(),
      },
    },
    async (args) => {
      const w = svc.state.getWorker(args.workerId);
      if (!w) {
        return { content: [{ type: "text", text: `Worker '${args.workerId}' not found.` }], isError: true };
      }
      if (!existsSync(w.streamLogPath)) {
        return { content: [{ type: "text", text: `Log file not yet created: ${w.streamLogPath}` }] };
      }
      const all = readFileSync(w.streamLogPath, "utf-8").split("\n").filter(Boolean);
      const lines = args.lines ?? 50;
      const tail = all.slice(-lines).join("\n");
      return { content: [{ type: "text", text: tail || "(empty)" }] };
    },
  );

  server.registerTool(
    "fleet_read_status",
    {
      description: "Read the current STATUS.md for a project (latest worker snapshot).",
      inputSchema: {
        project: z.string(),
      },
    },
    async (args) => {
      const p = svc.state.getProject(args.project);
      if (!p) {
        return { content: [{ type: "text", text: `Project '${args.project}' not found.` }], isError: true };
      }
      const statusPath = join(p.dir, "STATUS.md");
      if (!existsSync(statusPath)) {
        return { content: [{ type: "text", text: `(no STATUS.md yet at ${statusPath})` }] };
      }
      return {
        content: [{ type: "text", text: readFileSync(statusPath, "utf-8") }],
      };
    },
  );

  server.registerTool(
    "fleet_pending_signals",
    {
      description:
        "Drain queued urgent signals from workers (info/warn/halt). Call this periodically. " +
        "Each signal is delivered to the manager exactly once.",
      inputSchema: {},
    },
    async () => {
      const signals = svc.state.drainSignals();
      if (signals.length === 0) {
        return { content: [{ type: "text", text: "No pending signals." }] };
      }
      const lines = signals.map(
        (s) =>
          `[${s.urgency.toUpperCase()}] worker=${s.workerId} at=${new Date(s.postedAt).toISOString()}\n  ${s.message}`,
      );
      return { content: [{ type: "text", text: lines.join("\n\n") }] };
    },
  );
}

// Reconcile: scan ${root} for project dirs whose .hike-project.json is on
// disk but no `projects` row exists yet. Insert and return the newest one.
async function reconcileNewProjects(svc: FleetServices) {
  const { readdirSync, statSync } = await import("node:fs");
  const entries = readdirSync(svc.root, { withFileTypes: true });
  let newest: { name: string; groupName: string; planPath: string; dir: string } | null = null;
  let newestMtime = 0;
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (entry.name.startsWith(".")) continue;
    const projectDir = join(svc.root, entry.name);
    const metaPath = join(projectDir, ".hike-project.json");
    try {
      const metaContent = readFileSync(metaPath, "utf-8");
      const meta = JSON.parse(metaContent) as { group: string };
      // Mark as fleet-managed by writing .hike-fleet.json.
      const markerPath = join(projectDir, ".hike-fleet.json");
      if (!existsSync(markerPath)) {
        appendFileSync(markerPath, JSON.stringify({ managed: true, createdAt: new Date().toISOString() }, null, 2) + "\n");
      }
      // Upsert into DB.
      svc.state.upsertProject({
        name: entry.name,
        groupName: meta.group,
        dir: projectDir,
        planPath: join(projectDir, "PLAN.md"),
        managed: true,
        state: "idle",
      });
      const mtime = statSync(metaPath).mtimeMs;
      if (mtime > newestMtime) {
        newestMtime = mtime;
        newest = {
          name: entry.name,
          groupName: meta.group,
          planPath: join(projectDir, "PLAN.md"),
          dir: projectDir,
        };
      }
    } catch {
      // Not a hike project dir — skip.
    }
  }
  return newest;
}

function appendProjectToProjectsMD(path: string, name: string, group: string) {
  // Append the project under the "## Active" header. If PROJECTS.md doesn't
  // exist or has no Active section, create one.
  try {
    let content = existsSync(path) ? readFileSync(path, "utf-8") : "";
    if (!content.includes("## Active")) {
      content += "\n## Active\n\n";
    }
    const entry = `- [ ] **${name}** (group: ${group})\n`;
    if (content.endsWith("\n")) {
      content += entry;
    } else {
      content += "\n" + entry;
    }
    appendFileSync(path, ""); // ensure exists
    // We use a full rewrite to insert in the right place. For simplicity,
    // append at the end of Active block. v1 will do proper round-trip parsing.
    const idx = content.indexOf("## Active");
    if (idx >= 0) {
      const beforeActive = content.slice(0, idx + "## Active".length);
      const rest = content.slice(idx + "## Active".length);
      // Insert entry just after the Active header (and any blank lines).
      const trimmed = rest.replace(/^(\s*\n)+/, "\n");
      content = beforeActive + "\n" + entry + trimmed;
    }
    Bun.write(path, content);
  } catch (err) {
    log.warn("appendProjectToProjectsMD failed", { path, err: String(err) });
  }
}
