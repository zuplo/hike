// AsyncLocalStorage that lets MCP tool handlers know which worker (if any)
// is on the other end of the request. Set per HTTP request from headers;
// reset on response.
import { AsyncLocalStorage } from "node:async_hooks";

export type McpCallContext = {
  surface: "manager" | "worker";
  workerId?: string;
};

const als = new AsyncLocalStorage<McpCallContext>();

export function runWithMcpContext<T>(ctx: McpCallContext, fn: () => Promise<T>): Promise<T> {
  return als.run(ctx, fn);
}

export function currentMcpContext(): McpCallContext | undefined {
  return als.getStore();
}

export function requireWorkerId(): string {
  const ctx = als.getStore();
  if (!ctx || !ctx.workerId) {
    throw new Error("worker MCP request missing x-hike-worker-id header");
  }
  return ctx.workerId;
}
