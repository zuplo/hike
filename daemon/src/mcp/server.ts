// MCP HTTP server. One Bun.serve dispatches to two MCP server instances —
// /manager/mcp (manager-facing tools) and /worker/mcp (worker-facing tools).
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { WebStandardStreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/webStandardStreamableHttp.js";
import { registerManagerTools } from "./manager-tools.ts";
import { registerWorkerTools } from "./worker-tools.ts";
import { runWithMcpContext } from "./context.ts";
import { log } from "../log.ts";
import type { FleetServices } from "./services.ts";

export type McpHttpServer = {
  port: number;
  url: { manager: string; worker: string };
  stop: () => Promise<void>;
};

export async function startMcpHttpServer(
  svc: FleetServices,
  preferredPort: number,
): Promise<McpHttpServer> {
  const managerMcp = new McpServer({ name: "hike-fleet", version: "0.1.0" });
  const workerMcp = new McpServer({ name: "hike-fleet-worker", version: "0.1.0" });

  registerManagerTools(managerMcp, svc);
  registerWorkerTools(workerMcp, svc);

  const managerTransport = new WebStandardStreamableHTTPServerTransport({
    sessionIdGenerator: undefined, // stateless — each request is independent
    enableJsonResponse: true,
  });
  const workerTransport = new WebStandardStreamableHTTPServerTransport({
    sessionIdGenerator: undefined,
    enableJsonResponse: true,
  });

  await managerMcp.connect(managerTransport);
  await workerMcp.connect(workerTransport);

  const server = Bun.serve({
    port: preferredPort,
    hostname: "127.0.0.1",
    idleTimeout: 240, // 4 min — accommodates long tool calls
    async fetch(req) {
      const url = new URL(req.url);
      switch (true) {
        case url.pathname === "/healthz":
          return new Response("ok\n", { status: 200 });
        case url.pathname.startsWith("/manager/mcp"):
          return runWithMcpContext({ surface: "manager" }, () => managerTransport.handleRequest(req));
        case url.pathname.startsWith("/worker/mcp"): {
          const workerId = req.headers.get("x-hike-worker-id") ?? undefined;
          return runWithMcpContext({ surface: "worker", workerId }, () =>
            workerTransport.handleRequest(req),
          );
        }
        default:
          return new Response("not found\n", { status: 404 });
      }
    },
  });

  const port = server.port ?? preferredPort;
  if (!port) throw new Error("Bun.serve did not allocate a port");
  const url = {
    manager: `http://127.0.0.1:${port}/manager/mcp`,
    worker: `http://127.0.0.1:${port}/worker/mcp`,
  };
  log.info("MCP HTTP server listening", { port, url });

  return {
    port,
    url,
    async stop() {
      try {
        server.stop(true);
      } catch (err) {
        log.warn("MCP server stop error", { err: String(err) });
      }
      await Promise.all([managerMcp.close(), workerMcp.close()]).catch(() => {});
    },
  };
}
