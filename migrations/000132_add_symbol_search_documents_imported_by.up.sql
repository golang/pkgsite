-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- ln_imported_by_count is used to rank symbol searches
-- imported_by_count is sorted
ALTER TABLE symbol_search_documents ADD COLUMN imported_by_count INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_symbol_search_documents_imported_by_count_desc ON symbol_search_documents (imported_by_count DESC);
ALTER TABLE symbol_search_documents ADD COLUMN ln_imported_by_count NUMERIC;
CREATE INDEX idx_symbol_search_documents_ln_imported_by_count_desc ON symbol_search_documents (ln_imported_by_count DESC);

-- When search_documents is updated, also update symbol_search_documents.imported_by_count.
CREATE OR REPLACE FUNCTION trigger_modify_symbol_search_documents_imported_by_count() RETURNS TRIGGER AS $$
BEGIN
    UPDATE symbol_search_documents
    SET imported_by_count=NEW.imported_by_count;
    RETURN NEW;
END;
$$ LANGUAGE PLPGSQL;
CREATE TRIGGER set_symbol_search_documents_imported_by_count AFTER INSERT OR UPDATE ON search_documents
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_symbol_search_documents_imported_by_count();

-- When imported_by_count is updated, also update ln_imported_by_count.
CREATE FUNCTION trigger_modify_ln_imported_by_count() RETURNS TRIGGER AS $$
BEGIN
    NEW.ln_imported_by_count = ln(exp(1)+NEW.imported_by_count);
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;
CREATE TRIGGER set_ln_imported_by_count BEFORE INSERT OR UPDATE ON symbol_search_documents
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_ln_imported_by_count();

END;
