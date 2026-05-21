-- Scope GitHub cache tables per repository so that running nexus from
-- different repos (e.g. nexus vs nova) does not mix their issues/PRs.
-- These are pure caches; dropping them causes a one-time re-fetch on
-- next startup with no data loss.
DROP TABLE IF EXISTS github_prs;
DROP TABLE IF EXISTS github_issues;

CREATE TABLE github_prs (
    number      INTEGER NOT NULL,
    repo_path   TEXT    NOT NULL DEFAULT '',
    title       TEXT    NOT NULL,
    branch      TEXT    NOT NULL,
    author      TEXT,
    state       TEXT,
    is_draft    BOOLEAN DEFAULT FALSE,
    labels      TEXT,   -- JSON array of label names
    synced_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (number, repo_path)
);

CREATE TABLE github_issues (
    number              INTEGER NOT NULL,
    repo_path           TEXT    NOT NULL DEFAULT '',
    title               TEXT    NOT NULL,
    state               TEXT,
    labels              TEXT,               -- JSON array of label names
    assignees           TEXT,               -- JSON array of login strings
    parent_number       INTEGER,
    sub_issue_numbers   TEXT DEFAULT '[]',
    synced_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (number, repo_path)
);
