-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE package_version_states (
	package_path TEXT NOT NULL,
	module_path TEXT NOT NULL,
	version TEXT NOT NULL,
	status INTEGER NOT NULL,
	error TEXT,
	created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
	updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
	PRIMARY KEY (package_path, module_path, version),
	FOREIGN KEY (module_path, version)
		REFERENCES module_version_states(module_path, version)
		ON DELETE CASCADE
);

COMMENT ON TABLE package_version_states IS
'TABLE package_version_states is used to record the state of every package we have seen from the proxy.';

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON package_version_states
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON package_version_states IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

END;
