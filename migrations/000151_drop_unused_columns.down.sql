-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_names ADD COLUMN tsv_name_tokens TSVECTOR;
ALTER TABLE symbol_search_documents ADD COLUMN tsv_symbol_tokens TSVECTOR;

CREATE TRIGGER set_symbol_names_tsv_name_tokens
BEFORE INSERT OR UPDATE ON symbol_names
FOR EACH ROW EXECUTE FUNCTION set_tsv_name_tokens();

END;
