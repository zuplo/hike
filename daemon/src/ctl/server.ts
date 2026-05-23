// Unix-domain socket JSON-RPC 2.0 server. Each connection is a peer that
// may send multiple requests (line-delimited); we respond per request in
// order. No streaming responses in v0.
import { unlinkSync, existsSync } from "node:fs";
import type { Socket } from "bun";
import { methods, type MethodContext } from "./methods.ts";
import { RpcRequest, type RpcResponse } from "../types.ts";
import { log } from "../log.ts";

export type CtlServer = {
  close: () => void;
};

export function startCtlServer(socketPath: string, ctx: MethodContext): CtlServer {
  // Clean any stale socket file from a previous crash.
  if (existsSync(socketPath)) {
    try {
      unlinkSync(socketPath);
    } catch (err) {
      log.warn("could not remove stale socket", { socketPath, err: String(err) });
    }
  }

  type ConnState = { buf: string };

  const server = Bun.listen<ConnState>({
    unix: socketPath,
    socket: {
      open(socket: Socket<ConnState>) {
        socket.data = { buf: "" };
      },
      data(socket: Socket<ConnState>, chunk: Buffer | Uint8Array | string) {
        const str = typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk);
        socket.data.buf += str;
        // Process complete lines.
        for (;;) {
          const nl = socket.data.buf.indexOf("\n");
          if (nl < 0) break;
          const line = socket.data.buf.slice(0, nl).trim();
          socket.data.buf = socket.data.buf.slice(nl + 1);
          if (!line) continue;
          void handleLine(line, socket, ctx);
        }
      },
      error(_socket, err) {
        log.warn("ctl socket error", { err: String(err) });
      },
    },
  });

  log.info("ctl.sock listening", { socketPath });
  return {
    close() {
      try {
        server.stop(true);
      } catch (err) {
        log.warn("ctl server stop error", { err: String(err) });
      }
      try {
        unlinkSync(socketPath);
      } catch {
        // ignore
      }
    },
  };
}

async function handleLine(line: string, socket: Socket<{ buf: string }>, ctx: MethodContext) {
  let req: ReturnType<typeof RpcRequest.parse>;
  try {
    const raw = JSON.parse(line);
    req = RpcRequest.parse(raw);
  } catch (err) {
    const resp: RpcResponse = {
      jsonrpc: "2.0",
      id: null,
      error: { code: -32700, message: "Parse error", data: String(err) },
    };
    socket.write(JSON.stringify(resp) + "\n");
    return;
  }
  const id = req.id ?? null;
  const handler = methods[req.method];
  if (!handler) {
    const resp: RpcResponse = {
      jsonrpc: "2.0",
      id,
      error: { code: -32601, message: `Method not found: ${req.method}` },
    };
    socket.write(JSON.stringify(resp) + "\n");
    return;
  }
  try {
    const result = await handler(req.params, ctx);
    const resp: RpcResponse = { jsonrpc: "2.0", id, result };
    socket.write(JSON.stringify(resp) + "\n");
  } catch (err) {
    log.error("rpc handler error", { method: req.method, err: String(err) });
    const resp: RpcResponse = {
      jsonrpc: "2.0",
      id,
      error: { code: -32000, message: err instanceof Error ? err.message : String(err) },
    };
    socket.write(JSON.stringify(resp) + "\n");
  }
}
