-- Cache the issue body so cached reads render the description instead of
-- "(no description)", and clear the GitHub cache once so rows written under
-- inconsistent repo_path spellings (mixed path separators for the same
-- repository) are re-fetched under a single normalized key.
--
-- These tables are pure caches; clearing them causes a one-time re-fetch on
-- the next sync with no data loss.
ALTER TABLE github_issues ADD COLUMN body TEXT;

DELETE FROM github_issues;
DELETE FROM github_prs;
