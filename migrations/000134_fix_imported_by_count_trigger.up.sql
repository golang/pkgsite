-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION trigger_modify_symbol_search_documents_imported_by_count() RETURNS TRIGGER AS $$
BEGIN
    UPDATE symbol_search_documents ssd
    SET imported_by_count=NEW.imported_by_count
    WHERE ssd.unit_id=NEW.unit_id;
    RETURN NEW;
END;
$$ LANGUAGE PLPGSQL;

END;
