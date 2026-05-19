-- Simplify active_sessions: drop agent_pid, reduce status set to active/dead.
-- Session tracking is now purely "is a terminal open at this worktree?" —
-- agent process health is not tracked separately.
--
-- NOTE: this migration intentionally drops and recreates the table, which
-- destroys any existing session rows. Sessions are ephemeral runtime state
-- (PIDs become stale after a restart anyway), so data loss is acceptable here.
DROP TABLE IF EXISTS active_sessions;
CREATE TABLE active_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    worktree_path TEXT NOT NULL UNIQUE,
    shell_pid     INTEGER,
    agent_name    TEXT,
    prompt        TEXT,
    status        TEXT DEFAULT 'active' CHECK(status IN ('active','dead')),
    started_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
