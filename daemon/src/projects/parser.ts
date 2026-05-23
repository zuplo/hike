// Strict-but-forgiving PROJECTS.md parser. Extracts `- [status] **name** ...`
// entries from any section, ignores everything else, never alters user prose.
//
// Supported line shapes (whitespace tolerant, case-insensitive status):
//   - [ ] **platform-foo** — description
//   - [~] **platform-foo** (group: platform) — description
//   - [x] **platform-foo** — done
//
// Statuses: " " (idle), "~" (running), "!" (blocked), "x"/"X" (archived).
//
// Anything that doesn't match the regex is preserved by leaving the source
// untouched — this parser is read-only.

export type ProjectsMDStatus = "idle" | "running" | "blocked" | "archived" | "unknown";

export type ProjectsMDEntry = {
  /** 0-indexed source line. */
  line: number;
  raw: string;
  name: string;
  status: ProjectsMDStatus;
  /** Optional explicit group from "(group: foo)" parenthetical. */
  group?: string;
  /** Optional plan path from "plan: ./foo.md" mention. */
  plan?: string;
  /** Everything after the description separator (—, --, or :). */
  description?: string;
};

const STATUS_MAP: Record<string, ProjectsMDStatus> = {
  " ": "idle",
  "~": "running",
  "!": "blocked",
  x: "archived",
  X: "archived",
};

// Match e.g.: "- [ ] **name** (group: foo) — desc"
const ENTRY_RE = /^\s*[-*]\s*\[(.)\]\s*\*\*([^*]+)\*\*(?:\s*\(([^)]*)\))?\s*(?:[—\-:]+\s*(.*))?$/;

/**
 * Parse a PROJECTS.md document. Returns one entry per recognized bullet line.
 */
export function parseProjectsMD(content: string): ProjectsMDEntry[] {
  const entries: ProjectsMDEntry[] = [];
  const lines = content.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? "";
    const m = ENTRY_RE.exec(line);
    if (!m) continue;
    const [, statusChar = " ", nameRaw = "", parens = "", desc = ""] = m;
    const status = STATUS_MAP[statusChar] ?? "unknown";
    const name = nameRaw.trim();
    if (!name) continue;
    const entry: ProjectsMDEntry = {
      line: i,
      raw: line,
      name,
      status,
    };
    // Look for "group: foo" inside parens.
    if (parens) {
      const groupMatch = /group\s*:\s*([A-Za-z0-9_-]+)/.exec(parens);
      if (groupMatch) entry.group = groupMatch[1];
    }
    if (desc) {
      entry.description = desc.trim();
      const planMatch = /plan\s*:\s*([^\s,]+)/.exec(desc);
      if (planMatch) entry.plan = planMatch[1];
    }
    entries.push(entry);
  }
  return entries;
}

/**
 * Diff one parse against a set of (name, status) tuples representing the DB.
 * Returns three lists of names: added, removed, changed (status differs).
 */
export function diffEntries(
  file: ProjectsMDEntry[],
  db: { name: string; status: ProjectsMDStatus }[],
): { added: string[]; removed: string[]; changed: { name: string; from: ProjectsMDStatus; to: ProjectsMDStatus }[] } {
  const fileMap = new Map(file.map((e) => [e.name, e.status]));
  const dbMap = new Map(db.map((e) => [e.name, e.status]));
  const added: string[] = [];
  const removed: string[] = [];
  const changed: { name: string; from: ProjectsMDStatus; to: ProjectsMDStatus }[] = [];
  for (const [name, status] of fileMap) {
    const prev = dbMap.get(name);
    if (prev === undefined) {
      added.push(name);
    } else if (prev !== status) {
      changed.push({ name, from: prev, to: status });
    }
  }
  for (const [name, status] of dbMap) {
    if (!fileMap.has(name)) {
      removed.push(name);
    }
    void status;
  }
  return { added, removed, changed };
}
