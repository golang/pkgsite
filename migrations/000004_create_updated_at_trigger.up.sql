ALTER TABLE versions ALTER COLUMN updated_at SET DEFAULT current_timestamp;

CREATE OR REPLACE FUNCTION trigger_modify_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_updated_at
BEFORE INSERT OR UPDATE ON versions
FOR EACH ROW
EXECUTE PROCEDURE trigger_modify_updated_at();