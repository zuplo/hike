// Worker subprocess supervision via Claude Agent SDK query().
// Each WorkerHandle owns one Agent SDK conversation that runs to completion
// (or is aborted). We pipe SDK messages into a per-worker .jsonl log, mirror
// state into SQLite, and write STATUS.md / JOURNAL.md via the journal writer.
import { query, type SDKMessage } from "@anthropic-ai/claude-agent-sdk";
import { appendFileSync } from "node:fs";
import { ulid } from "ulid";
import type { State, WorkerState } from "../state/db.ts";
import { writeJournalEntry, writeStatus } from "../journal/writer.ts";
import { log } from "../log.ts";

export type WorkerHandle = {
  id: string;
  projectName: string;
  cwd: string;
  model: string;
  permissionMode: string;
  state: WorkerState;
  startedAt: number;
  abort: AbortController;
  /** Resolves when the SDK iterator finishes (success or abort). */
  done: Promise<void>;
  streamLogPath: string;
};

export type SpawnOpts = {
  /** Optional worker ID; one is generated if absent. Letting callers pass
   *  it lets them derive the log path before spawn. */
  id?: string;
  projectName: string;
  projectDir: string;
  prompt: string;
  model: string;
  permissionMode: "default" | "acceptEdits" | "plan" | "bypassPermissions";
  allowedTools: string[];
  disallowedTools: string[];
  workerMcpUrl: string;
  maxTurns?: number;
  workerLogPath: string; // absolute path to write the .jsonl stream
  appendSystemPrompt?: string;
};

export type SpawnDeps = {
  state: State;
  onComplete?: (handle: WorkerHandle, outcome: "success" | "error" | "stop_requested") => void;
};

/** Generate a worker ID (short ulid). Callers pre-generate so they can derive
 *  the log path before spawnWorker is called. */
export function newWorkerId(): string {
  return ulid();
}

/**
 * spawnWorker fires off a worker Agent SDK query() bound to a project dir.
 * Returns immediately with a handle; the SDK iterator runs to completion in
 * the background.
 */
export function spawnWorker(opts: SpawnOpts, deps: SpawnDeps): WorkerHandle {
  const id = opts.id ?? ulid();
  const startedAt = Date.now();
  const abort = new AbortController();

  const handle: WorkerHandle = {
    id,
    projectName: opts.projectName,
    cwd: opts.projectDir,
    model: opts.model,
    permissionMode: opts.permissionMode,
    state: "starting",
    startedAt,
    abort,
    done: Promise.resolve(), // replaced below
    streamLogPath: opts.workerLogPath,
  };

  // Persist initial worker row.
  deps.state.insertWorker({
    id,
    projectName: opts.projectName,
    sessionId: null,
    pid: null,
    cmd: `claude (Agent SDK query) model=${opts.model}`,
    model: opts.model,
    permissionMode: opts.permissionMode,
    state: "starting",
    exitCode: null,
    startedAt,
    endedAt: null,
    lastHeartbeat: startedAt,
    streamLogPath: opts.workerLogPath,
  });
  deps.state.setProjectState(opts.projectName, "running", id);

  handle.done = runWorker(handle, opts, deps).catch((err) => {
    log.error("worker run failed", { id, err: String(err) });
  });

  return handle;
}

async function runWorker(handle: WorkerHandle, opts: SpawnOpts, deps: SpawnDeps): Promise<void> {
  const { state } = deps;
  const id = handle.id;

  appendLogEvent(opts.workerLogPath, {
    type: "fleet.worker.start",
    workerId: id,
    project: opts.projectName,
    model: opts.model,
    permissionMode: opts.permissionMode,
    cwd: opts.projectDir,
    startedAt: handle.startedAt,
  });

  let outcome: "success" | "error" | "stop_requested" = "success";
  try {
    state.updateWorker(id, { state: "running", lastHeartbeat: Date.now() });
    handle.state = "running";

    const iterator = query({
      prompt: opts.prompt,
      options: {
        cwd: opts.projectDir,
        model: opts.model,
        permissionMode: opts.permissionMode,
        allowedTools: opts.allowedTools,
        disallowedTools: opts.disallowedTools,
        abortController: handle.abort,
        includePartialMessages: false,
        maxTurns: opts.maxTurns,
        mcpServers: {
          "hike-fleet": {
            type: "http",
            url: opts.workerMcpUrl,
            headers: { "x-hike-worker-id": id },
          },
        },
        systemPrompt: opts.appendSystemPrompt
          ? { type: "preset", preset: "claude_code", append: opts.appendSystemPrompt }
          : undefined,
        stderr: (data) => appendLogEvent(opts.workerLogPath, { type: "fleet.stderr", text: data }),
      },
    });

    for await (const msg of iterator) {
      // Mirror every SDK message into the .jsonl for `hike fleet logs`.
      appendLogEvent(opts.workerLogPath, msg as Record<string, unknown>);
      state.updateWorker(id, { lastHeartbeat: Date.now() });

      // Capture session id from the init system message.
      const m = msg as SDKMessage & { session_id?: string; subtype?: string };
      if (m.type === "system" && m.subtype === "init" && m.session_id) {
        state.updateWorker(id, { sessionId: m.session_id });
      }

      // Surface assistant text as journal entries (lightweight progress trail).
      if (m.type === "assistant" && "message" in m) {
        const content = (m.message as { content?: { type: string; text?: string }[] }).content;
        if (Array.isArray(content)) {
          const text = content
            .filter((c) => c.type === "text")
            .map((c) => c.text ?? "")
            .join("\n")
            .trim();
          if (text) {
            writeJournalEntry(opts.projectDir, `[worker ${id}] ${text.slice(0, 500)}`);
          }
        }
      }
    }

    // Iterator finished naturally — success.
    const endedAt = Date.now();
    state.updateWorker(id, { state: "done", endedAt });
    state.setProjectState(opts.projectName, "idle", null);
    handle.state = "done";
    appendLogEvent(opts.workerLogPath, {
      type: "fleet.worker.end",
      workerId: id,
      outcome: "success",
      endedAt,
    });
    writeStatus(opts.projectDir, `Worker ${id} finished successfully at ${new Date(endedAt).toISOString()}`);
  } catch (err) {
    const endedAt = Date.now();
    const aborted = handle.abort.signal.aborted;
    outcome = aborted ? "stop_requested" : "error";
    state.updateWorker(id, {
      state: aborted ? "killed" : "failed",
      endedAt,
    });
    state.setProjectState(opts.projectName, aborted ? "idle" : "blocked", null);
    handle.state = aborted ? "killed" : "failed";
    appendLogEvent(opts.workerLogPath, {
      type: "fleet.worker.end",
      workerId: id,
      outcome,
      error: String(err),
      endedAt,
    });
    writeStatus(opts.projectDir, `Worker ${id} ${outcome}: ${String(err).slice(0, 500)}`);
    log.warn("worker ended abnormally", { id, outcome, err: String(err) });
  } finally {
    deps.onComplete?.(handle, outcome);
  }
}

function appendLogEvent(path: string, event: Record<string, unknown>): void {
  try {
    appendFileSync(path, JSON.stringify(event) + "\n");
  } catch (err) {
    log.warn("worker log append failed", { path, err: String(err) });
  }
}
