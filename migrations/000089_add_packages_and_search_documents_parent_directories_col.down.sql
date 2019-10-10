-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE packages DROP COLUMN tsv_parent_directories;
ALTER TABLE search_documents DROP COLUMN tsv_parent_directories;

DROP FUNCTION IF EXISTS to_tsvector_parent_directories CASCADE;
DROP FUNCTION IF EXISTS trigger_modify_search_documents_tsv_parent_directories CASCADE;
DROP FUNCTION IF EXISTS trigger_modify_packages_tsv_parent_directories CASCADE;

END;
