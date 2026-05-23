-- Initial schema for hike-fleetd state.db.
-- Runs once on first open; future migrations live below as `-- @migration: N`.

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);
INSERT OR IGNORE INTO schema_version (version) VALUES (1);

CREATE TABLE IF NOT EXISTS projects (
    name TEXT PRIMARY KEY,
    group_name TEXT NOT NULL,
    dir TEXT NOT NULL,
    plan_path TEXT NOT NULL,
    managed INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'idle',     -- idle | running | blocked | done | archived
    current_run TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,                    -- ulid (short)
    project_name TEXT NOT NULL REFERENCES projects(name) ON DELETE CASCADE,
    session_id TEXT,                        -- claude session id from SDK system/init
    pid INTEGER,                            -- claude subprocess pid (best effort)
    cmd TEXT NOT NULL,                      -- argv summary, for the audit log
    model TEXT,
    permission_mode TEXT,
    state TEXT NOT NULL,                    -- starting|running|stopping|done|failed|killed|crashed
    exit_code INTEGER,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    last_heartbeat INTEGER,
    stream_log_path TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workers_project_state ON workers(project_name, state);
CREATE INDEX IF NOT EXISTS idx_workers_started_at ON workers(started_at DESC);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    project_name TEXT NOT NULL,
    prompt_hash TEXT,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    tool_calls INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL NOT NULL DEFAULT 0,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    outcome TEXT,                           -- success|stop_requested|timeout|error|crash
    error_msg TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_name, started_at DESC);

CREATE TABLE IF NOT EXISTS status_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_name TEXT NOT NULL,
    worker_id TEXT NOT NULL,
    summary TEXT NOT NULL,
    progress REAL,
    blockers_json TEXT,                     -- JSON array
    next_step TEXT,
    posted_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_status_project_time ON status_reports(project_name, posted_at DESC);

CREATE TABLE IF NOT EXISTS findings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_name TEXT NOT NULL,
    worker_id TEXT NOT NULL,
    kind TEXT NOT NULL,                     -- decision|question|blocker|warning|tool_result
    text TEXT NOT NULL,
    file TEXT,
    line INTEGER,
    posted_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_findings_project_time ON findings(project_name, posted_at DESC);

CREATE TABLE IF NOT EXISTS inbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    from_who TEXT NOT NULL,                 -- 'manager' or '<other_worker_id>'
    body TEXT NOT NULL,
    queued_at INTEGER NOT NULL,
    delivered_at INTEGER,
    acked INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_inbox_worker_undelivered ON inbox(worker_id, delivered_at);

CREATE TABLE IF NOT EXISTS pending_signals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    urgency TEXT NOT NULL,                  -- info|warn|halt
    message TEXT NOT NULL,
    posted_at INTEGER NOT NULL,
    drained_at INTEGER                      -- null until manager drains via fleet_pending_signals
);
CREATE INDEX IF NOT EXISTS idx_signals_undrained ON pending_signals(drained_at, posted_at);
