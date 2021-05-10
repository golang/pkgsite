-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE symbol_search_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    package_path_id INTEGER NOT NULL,
    symbol_name_id INTEGER NOT NULL,
    build_contexts TEXT[] NOT NULL,
    tsv_symbol_tokens TSVECTOR NOT NULL,

    UNIQUE(package_path_id, symbol_name_id),
    FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE

    -- Ideally this FK would be added now, but we need to populate
    -- search_documents.package_path_id first.
    -- FOREIGN KEY (package_path_id) REFERENCES search_documents(package_path_id) ON DELETE CASCADE,
);

CREATE INDEX idx_symbols_search_documents_tsv_symbol_tokens ON symbol_search_documents
    USING gin (tsv_symbol_tokens);

END;
