-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE packages DROP COLUMN name;
ALTER TABLE packages RENAME COLUMN module_path TO name;
ALTER TABLE packages ALTER COLUMN synopsis SET NOT NULL;
ALTER TABLE packages RENAME CONSTRAINT packages_module_path_fkey TO packages_name_fkey;
CREATE UNIQUE INDEX packages_name_version_key ON packages (name, version);

ALTER TABLE version_logs RENAME COLUMN module_path TO name;

ALTER TABLE versions RENAME COLUMN module_path TO name;
ALTER TABLE versions RENAME CONSTRAINT versions_module_path_fkey TO versions_name_fkey;

ALTER TABLE dependencies RENAME COLUMN module_path TO name;
ALTER TABLE dependencies RENAME COLUMN dependency_path TO dependency_name;

ALTER TABLE modules RENAME COLUMN path TO name;
ALTER TABLE modules RENAME COLUMN series_path TO series_name;
ALTER TABLE modules RENAME CONSTRAINT modules_series_path_fkey TO modules_series_name_fkey;

ALTER TABLE series RENAME COLUMN path TO name;

END;
