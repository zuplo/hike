// Watch PROJECTS.md for edits. On change, parse + diff against the DB and
// log a summary. v0 is read-only: we surface diffs to the daemon log; the
// manager queries fleet_reload_projects_md (v1) to act on them.
import chokidar, { type FSWatcher } from "chokidar";
import { readFileSync, existsSync } from "node:fs";
import type { State } from "../state/db.ts";
import { parseProjectsMD, diffEntries, type ProjectsMDStatus } from "./parser.ts";
import { log } from "../log.ts";

export type ProjectsWatcher = {
  reload: () => void;
  close: () => Promise<void>;
};

export function startProjectsWatcher(projectsMD: string, state: State): ProjectsWatcher {
  let watcher: FSWatcher | null = null;
  let debounce: ReturnType<typeof setTimeout> | null = null;

  function reload() {
    if (!existsSync(projectsMD)) {
      log.debug("PROJECTS.md not found yet", { path: projectsMD });
      return;
    }
    try {
      const content = readFileSync(projectsMD, "utf-8");
      const entries = parseProjectsMD(content);
      const dbRows = state.listProjects().map((p) => ({
        name: p.name,
        status: stateToStatus(p.state),
      }));
      const diff = diffEntries(entries, dbRows);
      if (diff.added.length === 0 && diff.removed.length === 0 && diff.changed.length === 0) {
        log.debug("PROJECTS.md reload: no drift");
        return;
      }
      log.info("PROJECTS.md drift", diff);
      // v0: just log. v1 will surface via fleet_reload_projects_md MCP tool.
    } catch (err) {
      log.warn("PROJECTS.md reload failed", { err: String(err) });
    }
  }

  function trigger() {
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(reload, 250);
  }

  watcher = chokidar.watch(projectsMD, {
    ignoreInitial: false,
    awaitWriteFinish: { stabilityThreshold: 100, pollInterval: 50 },
  });
  watcher.on("add", trigger);
  watcher.on("change", trigger);
  watcher.on("error", (err) => log.warn("chokidar error", { err: String(err) }));

  return {
    reload,
    async close() {
      if (debounce) clearTimeout(debounce);
      await watcher?.close();
    },
  };
}

function stateToStatus(
  s: "idle" | "running" | "blocked" | "done" | "archived",
): ProjectsMDStatus {
  switch (s) {
    case "idle":
    case "done":
      return "idle";
    case "running":
      return "running";
    case "blocked":
      return "blocked";
    case "archived":
      return "archived";
  }
}
