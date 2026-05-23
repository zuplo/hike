// Shared runtime types and zod schemas for IPC messages.
// Mirrors the JSON-RPC method names defined in internal/fleet/ipc.go.
import { z } from "zod";

// JSON-RPC 2.0 wire frames
export const RpcRequest = z.object({
  jsonrpc: z.literal("2.0"),
  id: z.union([z.number(), z.string(), z.null()]).optional(),
  method: z.string(),
  params: z.unknown().optional(),
});
export type RpcRequest = z.infer<typeof RpcRequest>;

export type RpcResponse =
  | {
      jsonrpc: "2.0";
      id: number | string | null;
      result: unknown;
    }
  | {
      jsonrpc: "2.0";
      id: number | string | null;
      error: { code: number; message: string; data?: unknown };
    };

// Method names — must match internal/fleet/ipc.go constants exactly.
export const METHODS = {
  ping: "ping",
  statusSnapshot: "status.snapshot",
  workerLogsTail: "worker.logs.tail",
  workerDispatch: "worker.dispatch",
  workerStop: "worker.stop",
  projectsList: "projects.list",
  projectAdd: "projects.add",
  daemonStop: "daemon.stop",
} as const;

export const DaemonStopParams = z.object({
  graceSeconds: z.number().int().nonnegative().default(30),
  kill: z.boolean().default(false),
});
export type DaemonStopParams = z.infer<typeof DaemonStopParams>;

export const WorkerLogsTailParams = z.object({
  project: z.string().optional(),
  workerId: z.string().optional(),
  follow: z.boolean().default(false),
  lines: z.number().int().nonnegative().optional(),
});
export type WorkerLogsTailParams = z.infer<typeof WorkerLogsTailParams>;

export const WorkerDispatchParams = z.object({
  project: z.string(),
  prompt: z.string().optional(),
  planPath: z.string().optional(),
  model: z.string().optional(),
  permissionMode: z.string().optional(),
  maxTurns: z.number().int().positive().optional(),
});
export type WorkerDispatchParams = z.infer<typeof WorkerDispatchParams>;

export const ProjectAddParams = z.object({
  group: z.string().optional(),
  name: z.string().optional(),
  color: z.string().optional(),
  plan: z.string().optional(),
});
export type ProjectAddParams = z.infer<typeof ProjectAddParams>;

// Outbound snapshot — mirrors fleet.SnapshotResult on the Go side.
export type Snapshot = {
  daemonPid: number;
  uptimeSec: number;
  mcpUrl: string;
  numWorkers: number;
  projects: {
    name: string;
    group: string;
    dir: string;
    state: string;
    currentRun?: string;
  }[];
  workers: {
    id: string;
    projectName: string;
    state: string;
    model: string;
    startedAt: number;
    endedAt?: number;
    lastHeartbeat?: number;
  }[];
};
