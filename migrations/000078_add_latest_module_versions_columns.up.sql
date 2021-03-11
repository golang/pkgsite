-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE latest_module_versions ADD COLUMN status INTEGER DEFAULT 0 NOT NULL;

COMMENT ON COLUMN latest_module_versions.status IS
'COLUMN status holds the status of the operations used to determine latest versions.';

ALTER TABLE latest_module_versions
      ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE
      DEFAULT CURRENT_TIMESTAMP
      NOT NULL;

COMMENT ON COLUMN latest_module_versions.updated_at IS
'COLUMN updated_at tracks the time that the row was last changed.';

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON latest_module_versions
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

COMMENT ON TRIGGER set_updated_at ON latest_module_versions IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE INDEX idx_latest_module_versions_status ON latest_module_versions(status);

END;
