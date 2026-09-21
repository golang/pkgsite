-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE TRIGGER set_symbol_search_documents_imported_by_count
AFTER INSERT OR UPDATE ON search_documents
FOR EACH ROW
EXECUTE FUNCTION trigger_modify_symbol_search_documents_imported_by_count();

END;
