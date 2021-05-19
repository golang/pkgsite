-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- At this point, paths.big_id should be identical to paths.id for every row,
-- and all newly inserted rows.

-- First, point all the foreign-key constraints from id to big_id.
-- By specifying NOT VALID, the command completes quickly, because it skips validation.
-- We can do the validation later, outside this migration; see the next one.

ALTER TABLE symbol_history DROP CONSTRAINT "symbol_history_module_path_id_fkey";
ALTER TABLE symbol_history ADD CONSTRAINT "symbol_history_module_path_id_fkey"
	FOREIGN KEY (module_path_id) REFERENCES paths(big_id) ON DELETE CASCADE
	NOT VALID;

ALTER TABLE symbol_history DROP CONSTRAINT "symbol_history_package_path_id_fkey";
ALTER TABLE symbol_history ADD CONSTRAINT "symbol_history_package_path_id_fkey"
	FOREIGN KEY (package_path_id) REFERENCES paths(big_id) ON DELETE CASCADE
	NOT VALID;

ALTER TABLE symbol_search_documents DROP CONSTRAINT "symbol_search_documents_package_path_id_fkey";
ALTER TABLE symbol_search_documents ADD CONSTRAINT "symbol_search_documents_package_path_id_fkey"
	FOREIGN KEY (package_path_id) REFERENCES paths(big_id) ON DELETE CASCADE
	NOT VALID;

ALTER TABLE latest_module_versions DROP CONSTRAINT "latest_module_versions_module_path_id_fkey";
ALTER TABLE latest_module_versions ADD CONSTRAINT "latest_module_versions_module_path_id_fkey"
	FOREIGN KEY (module_path_id) REFERENCES paths(big_id) ON DELETE CASCADE
	NOT VALID;

-- Now we can rename big_id to id and give it the necessary properties.

ALTER TABLE paths DROP COLUMN id;
ALTER TABLE paths RENAME COLUMN big_id TO id;
ALTER TABLE paths ADD PRIMARY KEY (id);
ALTER TABLE paths ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;

DROP TRIGGER set_paths_big_id ON paths;

END;
