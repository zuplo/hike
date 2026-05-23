// STATUS.md / JOURNAL.md write-through cache. Daemon owns all writes;
// workers post via MCP tools that funnel here.
import { existsSync, appendFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { log } from "../log.ts";

/**
 * writeStatus overwrites STATUS.md in the project dir with the given content
 * plus a timestamp footer. STATUS.md is "what's happening right now".
 */
export function writeStatus(projectDir: string, content: string): void {
  const path = join(projectDir, "STATUS.md");
  const body = `${content}\n\n---\n_Last updated: ${new Date().toISOString()}_\n`;
  try {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, body);
  } catch (err) {
    log.warn("writeStatus failed", { path, err: String(err) });
  }
}

/**
 * writeJournalEntry appends to JOURNAL.md. JOURNAL is "what happened over
 * time" — never overwritten.
 */
export function writeJournalEntry(projectDir: string, entry: string): void {
  const path = join(projectDir, "JOURNAL.md");
  const ts = new Date().toISOString();
  const line = `- **${ts}** — ${entry.replace(/\n/g, " ")}\n`;
  try {
    if (!existsSync(path)) {
      writeFileSync(path, "# Journal\n\n");
    }
    appendFileSync(path, line);
  } catch (err) {
    log.warn("writeJournalEntry failed", { path, err: String(err) });
  }
}
