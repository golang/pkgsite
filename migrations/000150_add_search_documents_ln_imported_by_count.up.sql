-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- ln_imported_by_count is used to rank package and symbol searches.
ALTER TABLE search_documents ADD COLUMN ln_imported_by_count NUMERIC;
CREATE INDEX idx_search_documents_ln_imported_by_count_desc ON search_documents (ln_imported_by_count DESC);

CREATE TRIGGER set_ln_imported_by_count BEFORE INSERT OR UPDATE ON search_documents
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_ln_imported_by_count();

END;
