// State layer for hike-fleetd. SQLite (bun:sqlite) in WAL mode at
// ${root}/.hike/fleet/state.db. Daemon owns all writes; CLI subcommands open
// read-only when the daemon is down (rare; v0 doesn't expose that path).
import { Database } from "bun:sqlite";
import schemaSQL from "./schema.sql" with { type: "text" };

export type ProjectState =
  | "idle"
  | "running"
  | "blocked"
  | "done"
  | "archived";

export type WorkerState =
  | "starting"
  | "running"
  | "stopping"
  | "done"
  | "failed"
  | "killed"
  | "crashed";

export type RunOutcome =
  | "success"
  | "stop_requested"
  | "timeout"
  | "error"
  | "crash";

export type ProjectRow = {
  name: string;
  groupName: string;
  dir: string;
  planPath: string;
  managed: number;
  state: ProjectState;
  currentRun: string | null;
  createdAt: number;
  updatedAt: number;
};

export type WorkerRow = {
  id: string;
  projectName: string;
  sessionId: string | null;
  pid: number | null;
  cmd: string;
  model: string | null;
  permissionMode: string | null;
  state: WorkerState;
  exitCode: number | null;
  startedAt: number;
  endedAt: number | null;
  lastHeartbeat: number | null;
  streamLogPath: string;
};

export type RunRow = {
  id: string;
  workerId: string;
  projectName: string;
  promptHash: string | null;
  inputTokens: number;
  outputTokens: number;
  toolCalls: number;
  costUsd: number;
  startedAt: number;
  endedAt: number | null;
  outcome: RunOutcome | null;
  errorMsg: string | null;
};

export type StatusReportRow = {
  id: number;
  projectName: string;
  workerId: string;
  summary: string;
  progress: number | null;
  blockersJson: string | null;
  nextStep: string | null;
  postedAt: number;
};

export type FindingRow = {
  id: number;
  projectName: string;
  workerId: string;
  kind: string;
  text: string;
  file: string | null;
  line: number | null;
  postedAt: number;
};

export type PendingSignal = {
  id: number;
  workerId: string;
  urgency: "info" | "warn" | "halt";
  message: string;
  postedAt: number;
  drainedAt: number | null;
};

/**
 * State wraps a bun:sqlite Database with prepared statements and typed
 * helpers. One instance per daemon process.
 */
export class State {
  readonly db: Database;

  constructor(path: string, opts: { readonly?: boolean } = {}) {
    this.db = new Database(path, opts.readonly ? { readonly: true } : { create: true });
    if (!opts.readonly) {
      this.db.run("PRAGMA journal_mode = WAL;");
      this.db.run("PRAGMA busy_timeout = 5000;");
      this.db.run("PRAGMA foreign_keys = ON;");
      this.db.exec(schemaSQL);
    } else {
      this.db.run("PRAGMA busy_timeout = 5000;");
    }
  }

  close(): void {
    this.db.close();
  }

  now(): number {
    return Date.now();
  }

  // -- projects --------------------------------------------------------------

  upsertProject(p: {
    name: string;
    groupName: string;
    dir: string;
    planPath: string;
    managed?: boolean;
    state?: ProjectState;
  }): void {
    const now = this.now();
    this.db.run(
      `INSERT INTO projects (name, group_name, dir, plan_path, managed, state, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(name) DO UPDATE SET
         group_name = excluded.group_name,
         dir = excluded.dir,
         plan_path = excluded.plan_path,
         managed = excluded.managed,
         state = COALESCE(excluded.state, projects.state),
         updated_at = excluded.updated_at`,
      [
        p.name,
        p.groupName,
        p.dir,
        p.planPath,
        p.managed === false ? 0 : 1,
        p.state ?? "idle",
        now,
        now,
      ],
    );
  }

  listProjects(): ProjectRow[] {
    return this.db
      .query(
        `SELECT name, group_name as groupName, dir, plan_path as planPath,
                managed, state, current_run as currentRun,
                created_at as createdAt, updated_at as updatedAt
         FROM projects ORDER BY name`,
      )
      .all() as ProjectRow[];
  }

  getProject(name: string): ProjectRow | null {
    return (this.db
      .query(
        `SELECT name, group_name as groupName, dir, plan_path as planPath,
                managed, state, current_run as currentRun,
                created_at as createdAt, updated_at as updatedAt
         FROM projects WHERE name = ?`,
      )
      .get(name) ?? null) as ProjectRow | null;
  }

  setProjectState(name: string, state: ProjectState, currentRun?: string | null): void {
    const now = this.now();
    if (currentRun !== undefined) {
      this.db.run(
        `UPDATE projects SET state = ?, current_run = ?, updated_at = ? WHERE name = ?`,
        [state, currentRun, now, name],
      );
    } else {
      this.db.run(
        `UPDATE projects SET state = ?, updated_at = ? WHERE name = ?`,
        [state, now, name],
      );
    }
  }

  // -- workers ---------------------------------------------------------------

  insertWorker(w: WorkerRow): void {
    this.db.run(
      `INSERT INTO workers (id, project_name, session_id, pid, cmd, model,
                            permission_mode, state, exit_code, started_at,
                            ended_at, last_heartbeat, stream_log_path)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        w.id,
        w.projectName,
        w.sessionId,
        w.pid,
        w.cmd,
        w.model,
        w.permissionMode,
        w.state,
        w.exitCode,
        w.startedAt,
        w.endedAt,
        w.lastHeartbeat,
        w.streamLogPath,
      ],
    );
  }

  updateWorker(
    id: string,
    patch: Partial<
      Pick<
        WorkerRow,
        | "sessionId"
        | "pid"
        | "state"
        | "exitCode"
        | "endedAt"
        | "lastHeartbeat"
      >
    >,
  ): void {
    const fields: string[] = [];
    const args: (string | number | null)[] = [];
    if ("sessionId" in patch) {
      fields.push("session_id = ?");
      args.push(patch.sessionId ?? null);
    }
    if ("pid" in patch) {
      fields.push("pid = ?");
      args.push(patch.pid ?? null);
    }
    if ("state" in patch) {
      fields.push("state = ?");
      args.push(patch.state!);
    }
    if ("exitCode" in patch) {
      fields.push("exit_code = ?");
      args.push(patch.exitCode ?? null);
    }
    if ("endedAt" in patch) {
      fields.push("ended_at = ?");
      args.push(patch.endedAt ?? null);
    }
    if ("lastHeartbeat" in patch) {
      fields.push("last_heartbeat = ?");
      args.push(patch.lastHeartbeat ?? null);
    }
    if (fields.length === 0) return;
    args.push(id);
    this.db.run(`UPDATE workers SET ${fields.join(", ")} WHERE id = ?`, args);
  }

  getWorker(id: string): WorkerRow | null {
    return (this.db
      .query(
        `SELECT id, project_name as projectName, session_id as sessionId,
                pid, cmd, model, permission_mode as permissionMode, state,
                exit_code as exitCode, started_at as startedAt,
                ended_at as endedAt, last_heartbeat as lastHeartbeat,
                stream_log_path as streamLogPath
         FROM workers WHERE id = ?`,
      )
      .get(id) ?? null) as WorkerRow | null;
  }

  listWorkers(filter: { state?: WorkerState; project?: string } = {}): WorkerRow[] {
    const conds: string[] = [];
    const args: (string | number)[] = [];
    if (filter.state) {
      conds.push("state = ?");
      args.push(filter.state);
    }
    if (filter.project) {
      conds.push("project_name = ?");
      args.push(filter.project);
    }
    const where = conds.length ? `WHERE ${conds.join(" AND ")}` : "";
    return this.db
      .query(
        `SELECT id, project_name as projectName, session_id as sessionId,
                pid, cmd, model, permission_mode as permissionMode, state,
                exit_code as exitCode, started_at as startedAt,
                ended_at as endedAt, last_heartbeat as lastHeartbeat,
                stream_log_path as streamLogPath
         FROM workers ${where} ORDER BY started_at DESC`,
      )
      .all(...args) as WorkerRow[];
  }

  /** Workers that the daemon thought were live but may not be after restart. */
  liveOrTransitionalWorkers(): WorkerRow[] {
    return this.db
      .query(
        `SELECT id, project_name as projectName, session_id as sessionId,
                pid, cmd, model, permission_mode as permissionMode, state,
                exit_code as exitCode, started_at as startedAt,
                ended_at as endedAt, last_heartbeat as lastHeartbeat,
                stream_log_path as streamLogPath
         FROM workers WHERE state IN ('starting','running','stopping')`,
      )
      .all() as WorkerRow[];
  }

  // -- runs ------------------------------------------------------------------

  insertRun(r: RunRow): void {
    this.db.run(
      `INSERT INTO runs (id, worker_id, project_name, prompt_hash,
                         input_tokens, output_tokens, tool_calls, cost_usd,
                         started_at, ended_at, outcome, error_msg)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      [
        r.id,
        r.workerId,
        r.projectName,
        r.promptHash,
        r.inputTokens,
        r.outputTokens,
        r.toolCalls,
        r.costUsd,
        r.startedAt,
        r.endedAt,
        r.outcome,
        r.errorMsg,
      ],
    );
  }

  updateRun(
    id: string,
    patch: Partial<
      Pick<
        RunRow,
        | "inputTokens"
        | "outputTokens"
        | "toolCalls"
        | "costUsd"
        | "endedAt"
        | "outcome"
        | "errorMsg"
      >
    >,
  ): void {
    const fields: string[] = [];
    const args: (string | number | null)[] = [];
    for (const [k, v] of Object.entries(patch)) {
      const col =
        k === "inputTokens"
          ? "input_tokens"
          : k === "outputTokens"
          ? "output_tokens"
          : k === "toolCalls"
          ? "tool_calls"
          : k === "costUsd"
          ? "cost_usd"
          : k === "endedAt"
          ? "ended_at"
          : k === "errorMsg"
          ? "error_msg"
          : k;
      fields.push(`${col} = ?`);
      args.push(v as string | number | null);
    }
    if (fields.length === 0) return;
    args.push(id);
    this.db.run(`UPDATE runs SET ${fields.join(", ")} WHERE id = ?`, args);
  }

  // -- status_reports --------------------------------------------------------

  insertStatusReport(r: Omit<StatusReportRow, "id">): number {
    const stmt = this.db.query(
      `INSERT INTO status_reports
         (project_name, worker_id, summary, progress, blockers_json, next_step, posted_at)
       VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
    );
    const row = stmt.get(
      r.projectName,
      r.workerId,
      r.summary,
      r.progress,
      r.blockersJson,
      r.nextStep,
      r.postedAt,
    ) as { id: number };
    return row.id;
  }

  latestStatusReport(projectName: string): StatusReportRow | null {
    return (this.db
      .query(
        `SELECT id, project_name as projectName, worker_id as workerId,
                summary, progress, blockers_json as blockersJson,
                next_step as nextStep, posted_at as postedAt
         FROM status_reports WHERE project_name = ? ORDER BY posted_at DESC LIMIT 1`,
      )
      .get(projectName) ?? null) as StatusReportRow | null;
  }

  // -- findings --------------------------------------------------------------

  insertFinding(f: Omit<FindingRow, "id">): number {
    const stmt = this.db.query(
      `INSERT INTO findings
         (project_name, worker_id, kind, text, file, line, posted_at)
       VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
    );
    const row = stmt.get(
      f.projectName,
      f.workerId,
      f.kind,
      f.text,
      f.file,
      f.line,
      f.postedAt,
    ) as { id: number };
    return row.id;
  }

  // -- inbox -----------------------------------------------------------------

  enqueueInbox(workerId: string, from: string, body: string): number {
    const stmt = this.db.query(
      `INSERT INTO inbox (worker_id, from_who, body, queued_at) VALUES (?, ?, ?, ?) RETURNING id`,
    );
    const row = stmt.get(workerId, from, body, this.now()) as { id: number };
    return row.id;
  }

  drainInbox(workerId: string, sinceMs = 0): { id: number; from: string; body: string; queuedAt: number }[] {
    const rows = this.db
      .query(
        `SELECT id, from_who as fromWho, body, queued_at as queuedAt
         FROM inbox
         WHERE worker_id = ? AND delivered_at IS NULL AND queued_at > ?
         ORDER BY queued_at ASC`,
      )
      .all(workerId, sinceMs) as { id: number; fromWho: string; body: string; queuedAt: number }[];
    if (rows.length === 0) return [];
    const delivered = this.now();
    const idList = rows.map((r) => r.id);
    const placeholders = idList.map(() => "?").join(",");
    this.db.run(
      `UPDATE inbox SET delivered_at = ? WHERE id IN (${placeholders})`,
      [delivered, ...idList],
    );
    return rows.map((r) => ({ id: r.id, from: r.fromWho, body: r.body, queuedAt: r.queuedAt }));
  }

  // -- pending_signals -------------------------------------------------------

  enqueueSignal(workerId: string, urgency: "info" | "warn" | "halt", message: string): number {
    const stmt = this.db.query(
      `INSERT INTO pending_signals (worker_id, urgency, message, posted_at)
       VALUES (?, ?, ?, ?) RETURNING id`,
    );
    const row = stmt.get(workerId, urgency, message, this.now()) as { id: number };
    return row.id;
  }

  drainSignals(): PendingSignal[] {
    const rows = this.db
      .query(
        `SELECT id, worker_id as workerId, urgency, message, posted_at as postedAt, drained_at as drainedAt
         FROM pending_signals WHERE drained_at IS NULL ORDER BY posted_at ASC`,
      )
      .all() as PendingSignal[];
    if (rows.length === 0) return [];
    const now = this.now();
    const ids = rows.map((r) => r.id);
    const placeholders = ids.map(() => "?").join(",");
    this.db.run(
      `UPDATE pending_signals SET drained_at = ? WHERE id IN (${placeholders})`,
      [now, ...ids],
    );
    return rows;
  }
}
