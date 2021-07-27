-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_search_documents DROP COLUMN package_name;
ALTER TABLE symbol_search_documents DROP COLUMN package_path;
ALTER TABLE symbol_search_documents DROP COLUMN uuid_package_name;
ALTER TABLE symbol_search_documents DROP COLUMN uuid_package_path;

DROP TRIGGER set_uuid_package_name ON symbol_search_documents;
DROP TRIGGER set_uuid_package_path ON symbol_search_documents;
DROP FUNCTION trigger_modify_uuid_package_name;
DROP FUNCTION trigger_modify_uuid_package_path;

END;
