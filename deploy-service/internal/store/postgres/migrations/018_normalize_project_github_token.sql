ALTER TABLE projects
ADD COLUMN IF NOT EXISTS github_token_encrypted TEXT;

UPDATE projects
SET github_token_encrypted = ''
WHERE github_token_encrypted IS NULL;

ALTER TABLE projects
ALTER COLUMN github_token_encrypted SET DEFAULT '';

ALTER TABLE projects
ALTER COLUMN github_token_encrypted SET NOT NULL;
