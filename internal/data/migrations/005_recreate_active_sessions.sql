-- Recreate active_sessions with the full phase-4 schema.
-- The phase-2 table had a narrower schema (id, worktree_path, pid, started_at).
-- We drop it unconditionally so the correct columns are always present.
DROP TABLE IF EXISTS active_sessions;
CREATE TABLE active_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    worktree_path TEXT NOT NULL UNIQUE,
    shell_pid     INTEGER,
    agent_name    TEXT,
    agent_pid     INTEGER,
    prompt        TEXT,
    status        TEXT DEFAULT 'active' CHECK(status IN ('active','agent_running','agent_done','agent_failed','dead')),
    started_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
