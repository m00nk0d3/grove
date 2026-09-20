ALTER TABLE active_sessions ADD COLUMN runtime        TEXT NOT NULL DEFAULT 'local';
ALTER TABLE active_sessions ADD COLUMN runtime_id     TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_workspace_id TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_tab_id  TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_pane_id TEXT;
ALTER TABLE active_sessions ADD COLUMN workflow_run_id   TEXT;
ALTER TABLE active_sessions ADD COLUMN degraded_reason   TEXT;
