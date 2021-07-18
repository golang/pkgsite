-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION set_tsv_name_tokens() RETURNS TRIGGER AS $$
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

END;
