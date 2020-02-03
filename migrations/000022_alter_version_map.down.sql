-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE version_map ADD CONSTRAINT version_map_module_path_fkey
	FOREIGN KEY (module_path, resolved_version)
	REFERENCES versions(module_path, version);

ALTER TABLE version_map DROP COLUMN sort_version;

END;

