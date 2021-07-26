-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_symbol_search_documents_package_symbol_id;
ALTER TABLE symbol_search_documents DROP CONSTRAINT symbol_search_documents_package_symbol_id_fkey;
ALTER TABLE symbol_search_documents ALTER COLUMN package_symbol_id DROP NOT NULL;

END;
