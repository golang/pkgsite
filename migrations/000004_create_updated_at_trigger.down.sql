ALTER TABLE versions ALTER COLUMN updated_at DROP DEFAULT;
DROP TRIGGER IF EXISTS set_updated_at ON versions;
DROP FUNCTION IF EXISTS trigger_modify_updated_at();