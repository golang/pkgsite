-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TRIGGER set_tsv_parent_directories ON search_documents;
ALTER TABLE search_documents DROP COLUMN tsv_parent_directories;

DROP TRIGGER set_tsv_parent_directories ON packages;
ALTER TABLE packages DROP COLUMN tsv_parent_directories;

DROP FUNCTION trigger_modify_packages_tsv_parent_directories;
DROP FUNCTION to_tsvector_parent_directories;

END;
