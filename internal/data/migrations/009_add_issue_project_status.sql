-- Cache the GitHub Projects v2 "Status" field for each issue so the issues view
-- can report the board's own status (for example "In progress" or "Backlog")
-- instead of inferring progress from local worktrees alone.
--
-- The value is stored with the board's original wording. It is empty for issues
-- that are on no project board, and for repositories whose token lacks the
-- read:project scope.
ALTER TABLE github_issues ADD COLUMN project_status TEXT;
