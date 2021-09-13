-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TRIGGER set_symbol_names_tsv_name_tokens ON symbol_names;
ALTER TABLE symbol_names DROP COLUMN tsv_name_tokens;
ALTER TABLE symbol_search_documents DROP COLUMN tsv_symbol_tokens;

END;
