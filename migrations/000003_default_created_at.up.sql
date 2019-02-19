ALTER TABLE series ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE modules ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE versions ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE version_logs ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE documents ADD COLUMN created_at TIMESTAMP DEFAULT current_timestamp;
