-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents DROP COLUMN tsv_path_tokens;
ALTER TABLE symbol_names DROP COLUMN tsv_name_tokens;
DROP FUNCTION set_tsv_name_tokens;
DROP TRIGGER IF EXISTS set_symbol_names_tsv_name_tokens ON symbol_names;
ALTER TABLE symbol_search_documents DROP COLUMN package_symbol_id;
ALTER TABLE symbol_search_documents DROP COLUMN goos goos;
ALTER TABLE symbol_search_documents DROP COLUMN goarch goarch;

END;
