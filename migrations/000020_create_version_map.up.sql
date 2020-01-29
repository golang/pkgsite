-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE version_map (
	module_path TEXT NOT NULL,
	requested_version TEXT NOT NULL,
	resolved_version TEXT,
	status INTEGER NOT NULL,
	error TEXT,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (module_path, requested_version),
	FOREIGN KEY (module_path, resolved_version) REFERENCES versions(module_path, version)
);

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON version_map
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

COMMENT ON TABLE version_map IS
'TABLE version_map contains data about a user-requested path and the semantic version that it resolves to. It is used to support fetching frontend detail pages using module queries.';
COMMENT ON COLUMN version_map.resolved_version IS
'COLUMN resolved_version is the semantic version that a requested_version resolves to.';
COMMENT ON COLUMN version_map.requested_version IS
'COLUMN requested_version is the version that was requested by a user from the frontend. It may or may not resolve to a semantic version.';
COMMENT ON COLUMN version_map.status IS
'COLUMN status is the status returned by the ETL when fetching the module version.';
COMMENT ON COLUMN version_map.error IS
'COLUMN status is the error that occurred when fetching the module version, in cases when status != 200.';

END;
