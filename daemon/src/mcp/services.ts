// Service bundle handed to tool factories. Lets us keep the tool modules
// pure and easy to test.
import type { State } from "../state/db.ts";
import type { Supervisor } from "../supervisor/registry.ts";

export type FleetServices = {
  state: State;
  supervisor: Supervisor;
  /** Absolute path to the hike repo root (contains hike.yaml). */
  root: string;
  /** Path to PROJECTS.md (usually ${root}/PROJECTS.md). */
  projectsMD: string;
  /** Path to the worker prompt file (read for default appendSystemPrompt). */
  workerPromptFile: string;
  /** Worker MCP URL (handed to spawned workers). */
  workerMcpUrl: string;
  /** Default worker config from hike.yaml fleet block. */
  workerConfig: {
    model: string;
    permissionMode: "default" | "acceptEdits" | "plan" | "bypassPermissions";
    allowedTools: string[];
    disallowedTools: string[];
    maxTurns?: number;
  };
  /** Path to the worker log dir (${root}/.hike/fleet/logs/workers). */
  workerLogDir: string;
  /** Resolves the hike-fleetd's own root binary `hike` for shelling out. */
  hikeBinary: string;
};
