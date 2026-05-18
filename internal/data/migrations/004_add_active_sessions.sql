-- active_sessions: tracks live terminal sessions per worktree
CREATE TABLE IF NOT EXISTS active_sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    worktree_path TEXT NOT NULL UNIQUE,
    shell_pid     INTEGER,
    agent_name    TEXT,
    agent_pid     INTEGER,
    prompt        TEXT,
    status        TEXT DEFAULT 'active',
    started_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
