-- active_sessions: tracks live terminal sessions per worktree.
-- One row per worktree; shell_pid identifies the terminal process.
CREATE TABLE IF NOT EXISTS active_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    worktree_path TEXT NOT NULL UNIQUE,
    shell_pid     INTEGER,
    agent_name    TEXT,
    prompt        TEXT,
    status        TEXT DEFAULT 'active' CHECK(status IN ('active','dead')),
    started_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
