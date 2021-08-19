-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_symbol_search_documents_symbol_name_imported_by_count ON symbol_search_documents(lower(symbol_name), imported_by_count DESC);

END;
