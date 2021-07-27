-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_search_documents DROP COLUMN imported_by_count;
ALTER TABLE symbol_search_documents DROP COLUMN ln_imported_by_count;

DROP TRIGGER set_ln_imported_by_count ON symbol_search_documents;
DROP FUNCTION trigger_modify_ln_imported_by_count;
DROP TRIGGER set_symbol_search_documents_imported_by_count ON search_documents;
DROP FUNCTION trigger_modify_symbol_search_documents_imported_by_count;

END;
