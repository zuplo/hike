// Worker-facing MCP tools. Each call must carry x-hike-worker-id (set by the
// daemon on the worker's mcpServers config); we use AsyncLocalStorage to
// thread it through.
import { z } from "zod";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { FleetServices } from "./services.ts";
import { requireWorkerId } from "./context.ts";
import { writeStatus, writeJournalEntry } from "../journal/writer.ts";
import { log } from "../log.ts";

export function registerWorkerTools(server: McpServer, svc: FleetServices) {
  server.registerTool(
    "fleet_post_status",
    {
      description:
        "Post a status update for your current run. Overwrites STATUS.md in your project dir and " +
        "appends a JOURNAL.md entry. Call this whenever you finish a meaningful phase of work.",
      inputSchema: {
        summary: z.string().describe("Short summary of current state (1-3 sentences)."),
        progress: z.number().min(0).max(1).optional().describe("Optional 0..1 progress estimate."),
        blockers: z.array(z.string()).optional().describe("Anything blocking forward progress."),
        next: z.string().optional().describe("What you plan to do next."),
      },
    },
    async (args) => {
      const workerId = requireWorkerId();
      const w = svc.state.getWorker(workerId);
      if (!w) {
        return { content: [{ type: "text", text: `Worker ${workerId} not found.` }], isError: true };
      }
      svc.state.insertStatusReport({
        projectName: w.projectName,
        workerId,
        summary: args.summary,
        progress: args.progress ?? null,
        blockersJson: args.blockers && args.blockers.length > 0 ? JSON.stringify(args.blockers) : null,
        nextStep: args.next ?? null,
        postedAt: Date.now(),
      });
      const lines = [
        `# Status for ${w.projectName}`,
        ``,
        `**Summary:** ${args.summary}`,
        args.progress != null ? `**Progress:** ${Math.round(args.progress * 100)}%` : "",
        args.blockers && args.blockers.length > 0
          ? `**Blockers:**\n${args.blockers.map((b) => `- ${b}`).join("\n")}`
          : "",
        args.next ? `**Next:** ${args.next}` : "",
      ]
        .filter(Boolean)
        .join("\n");
      const p = svc.state.getProject(w.projectName);
      if (p) {
        writeStatus(p.dir, lines);
        writeJournalEntry(p.dir, `[${workerId}] STATUS — ${args.summary.slice(0, 200)}`);
      }
      svc.state.updateWorker(workerId, { lastHeartbeat: Date.now() });
      log.info("fleet_post_status", { workerId, project: w.projectName, progress: args.progress });
      return { content: [{ type: "text", text: "Status posted." }] };
    },
  );

  server.registerTool(
    "fleet_post_finding",
    {
      description:
        "Record a discrete finding: a decision made, a question raised, a blocker discovered, " +
        "or a warning. Persists to JOURNAL.md and the findings table.",
      inputSchema: {
        kind: z.enum(["decision", "question", "blocker", "warning", "tool_result"]),
        text: z.string(),
        file: z.string().optional(),
        line: z.number().int().positive().optional(),
      },
    },
    async (args) => {
      const workerId = requireWorkerId();
      const w = svc.state.getWorker(workerId);
      if (!w) {
        return { content: [{ type: "text", text: `Worker ${workerId} not found.` }], isError: true };
      }
      svc.state.insertFinding({
        projectName: w.projectName,
        workerId,
        kind: args.kind,
        text: args.text,
        file: args.file ?? null,
        line: args.line ?? null,
        postedAt: Date.now(),
      });
      const p = svc.state.getProject(w.projectName);
      if (p) {
        const loc = args.file ? ` (${args.file}${args.line ? ":" + args.line : ""})` : "";
        writeJournalEntry(p.dir, `[${workerId}] ${args.kind.toUpperCase()}${loc} — ${args.text.slice(0, 300)}`);
      }
      // If kind=blocker, also enqueue a halt signal for the manager.
      if (args.kind === "blocker") {
        svc.state.enqueueSignal(workerId, "halt", `${w.projectName}: ${args.text.slice(0, 200)}`);
      }
      return { content: [{ type: "text", text: "Finding recorded." }] };
    },
  );

  server.registerTool(
    "fleet_check_inbox",
    {
      description: "Poll for queued messages from the manager. Returns messages and marks them delivered.",
      inputSchema: {
        since: z.number().int().nonnegative().optional().describe("Only fetch messages newer than this epoch ms."),
      },
    },
    async (args) => {
      const workerId = requireWorkerId();
      const messages = svc.state.drainInbox(workerId, args.since ?? 0);
      if (messages.length === 0) {
        return { content: [{ type: "text", text: "(no messages)" }] };
      }
      const lines = messages.map(
        (m) => `[${new Date(m.queuedAt).toISOString()}] from=${m.from}\n  ${m.body}`,
      );
      return { content: [{ type: "text", text: lines.join("\n\n") }] };
    },
  );

  server.registerTool(
    "fleet_send_to_manager",
    {
      description:
        "Send a direct message to the manager. The manager will see this in the next " +
        "fleet_pending_signals drain. Use 'halt' urgency for blocking issues that need " +
        "human input before you can continue.",
      inputSchema: {
        message: z.string(),
        urgency: z.enum(["info", "warn", "halt"]).default("info"),
      },
    },
    async (args) => {
      const workerId = requireWorkerId();
      const w = svc.state.getWorker(workerId);
      svc.state.enqueueSignal(workerId, args.urgency, args.message);
      log.info("worker→manager", { workerId, urgency: args.urgency, project: w?.projectName });
      return { content: [{ type: "text", text: "Message queued for manager." }] };
    },
  );
}
