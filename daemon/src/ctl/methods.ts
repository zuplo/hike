// Handlers for JSON-RPC methods coming in over ctl.sock.
import type { State } from "../state/db.ts";
import type { Supervisor } from "../supervisor/registry.ts";
import { METHODS, type Snapshot, DaemonStopParams } from "../types.ts";
import { log } from "../log.ts";

export type MethodContext = {
  state: State;
  supervisor: Supervisor;
  pid: number;
  mcpUrl: string;
  startedAt: number;
  /** Resolves when shutdown finishes; ctl server can use this to drain. */
  requestShutdown: (graceMs: number, kill: boolean) => Promise<void>;
};

export type MethodHandler = (params: unknown, ctx: MethodContext) => Promise<unknown>;

export const methods: Record<string, MethodHandler> = {
  [METHODS.ping]: async () => ({ ok: true, pong: Date.now() }),

  [METHODS.statusSnapshot]: async (_params, ctx): Promise<Snapshot> => {
    const projects = ctx.state.listProjects();
    const workers = ctx.state.listWorkers();
    return {
      daemonPid: ctx.pid,
      uptimeSec: Math.round((Date.now() - ctx.startedAt) / 1000),
      mcpUrl: ctx.mcpUrl,
      numWorkers: workers.filter((w) => w.state === "running" || w.state === "starting").length,
      projects: projects.map((p) => ({
        name: p.name,
        group: p.groupName,
        dir: p.dir,
        state: p.state,
        currentRun: p.currentRun ?? undefined,
      })),
      workers: workers.map((w) => ({
        id: w.id,
        projectName: w.projectName,
        state: w.state,
        model: w.model ?? "",
        startedAt: w.startedAt,
        endedAt: w.endedAt ?? undefined,
        lastHeartbeat: w.lastHeartbeat ?? undefined,
      })),
    };
  },

  [METHODS.daemonStop]: async (params, ctx) => {
    const p = DaemonStopParams.parse(params ?? {});
    log.info("daemon.stop RPC received", { grace: p.graceSeconds, kill: p.kill });
    // Kick off shutdown async; respond immediately so the CLI sees a result.
    queueMicrotask(() => {
      ctx.requestShutdown(p.graceSeconds * 1000, p.kill).catch((err) => {
        log.error("shutdown failed", { err: String(err) });
      });
    });
    return { stopping: true };
  },
};
