-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE TRIGGER set_symbol_search_documents_imported_by_count
AFTER UPDATE OF imported_by_count ON search_documents
FOR EACH ROW
WHEN (OLD.imported_by_count IS DISTINCT FROM NEW.imported_by_count)
EXECUTE FUNCTION trigger_modify_symbol_search_documents_imported_by_count();

END;
