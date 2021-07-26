-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_search_documents ALTER COLUMN package_symbol_id SET NOT NULL;
CREATE INDEX idx_symbol_search_documents_package_symbol_id ON symbol_search_documents(package_symbol_id);
ALTER TABLE symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_package_symbol_id_fkey
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE;

END;
