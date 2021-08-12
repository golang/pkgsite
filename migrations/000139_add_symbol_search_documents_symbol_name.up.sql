-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_search_documents ADD COLUMN symbol_name TEXT;
CREATE INDEX idx_symbol_search_documents_lowercase_symbol_name ON symbol_search_documents(lower(symbol_name));

END;
