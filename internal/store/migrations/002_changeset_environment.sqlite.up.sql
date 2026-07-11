ALTER TABLE changesets ADD COLUMN environment_id TEXT REFERENCES environments(id);

UPDATE changesets
SET environment_id = (
  SELECT e.id FROM environments e
  WHERE e.project_id = changesets.project_id AND e.name = 'dev'
  LIMIT 1
)
WHERE status = 'open' AND (environment_id IS NULL OR environment_id = '');
