-- Add hierarchy columns to github_issues
ALTER TABLE github_issues ADD COLUMN parent_number INTEGER;
ALTER TABLE github_issues ADD COLUMN sub_issue_numbers TEXT DEFAULT '[]';
