-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents ADD COLUMN tsv_path_tokens TSVECTOR;

ALTER TABLE symbol_names ADD COLUMN tsv_name_tokens TSVECTOR;
CREATE FUNCTION set_tsv_name_tokens() RETURNS TRIGGER AS $$
BEGIN
    NEW.tsv_name_tokens =
        -- Index full identifier name.
        SETWEIGHT(TO_TSVECTOR('symbols', replace(NEW.name, '_', '-')), 'C') ||
        -- Index <identifier> without parent name (i.e. "Begin" in
        -- "DB.Begin").
        -- This is weighted less, so that if other symbols are just named
        -- "Begin" they will rank higher in a search for "Begin".
        SETWEIGHT(
            TO_TSVECTOR('symbols', split_part(replace(NEW.name, '_', '-'), '.', 2)),
            'C');
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;

CREATE TRIGGER set_symbol_names_tsv_name_tokens
BEFORE INSERT OR UPDATE ON symbol_names
FOR EACH ROW EXECUTE FUNCTION set_tsv_name_tokens();

ALTER TABLE symbol_search_documents ALTER COLUMN tsv_symbol_tokens DROP NOT NULL;
ALTER TABLE symbol_search_documents ADD COLUMN package_symbol_id BIGINT;
ALTER TABLE symbol_search_documents ADD COLUMN goos goos;
ALTER TABLE symbol_search_documents ADD COLUMN goarch goarch;

END;
