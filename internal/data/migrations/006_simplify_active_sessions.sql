-- Simplify active_sessions: drop agent_pid, reduce status set to active/dead.
-- Session tracking is now purely "is a terminal open at this worktree?" —
-- agent process health is not tracked separately.
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
