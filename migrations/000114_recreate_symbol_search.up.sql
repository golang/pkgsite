-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE symbol_search_documents;

CREATE TABLE symbol_search_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    package_path_id BIGINT NOT NULL,
    symbol_name_id BIGINT NOT NULL,
    unit_id BIGINT NOT NULL,
    tsv_symbol_tokens TSVECTOR NOT NULL,
    UNIQUE(package_path_id, symbol_name_id),
    -- Ideally this FK would be added now, but we need to populate search_documents columns first.
    -- FOREIGN KEY (package_path_id) REFERENCES search_documents(package_path_id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (unit_id) REFERENCES units(id) ON DELETE CASCADE
);

CREATE INDEX idx_symbols_search_documents_symbol_name_id ON symbol_search_documents(symbol_name_id);
CREATE INDEX idx_symbols_search_documents_tsv_symbol_tokens ON symbol_search_documents USING gin (tsv_symbol_tokens);
COMMENT ON TABLE symbol_search_documents IS
'TABLE symbol_search_documents contains data used to search for symbols. A row exists for the latest version of each package_path and each exported symbol in that package. Each symbol maps to a package in search_documents.';
COMMENT ON COLUMN symbol_search_documents.tsv_symbol_tokens IS
'COLUMN symbol_search_documents.tsv_symbol_tokens contains data used to search for symbols. It contains the TSVECTOR of <package>.<symbol>, <symbol>, and <recv> when <symbol> is of the form <type>.<recv>. If the symbol is a field or method, it also contains the identifier name without the parent name.';

ALTER TABLE search_documents DROP COLUMN package_path_id;
ALTER TABLE search_documents ADD COLUMN package_path_id BIGINT;
CREATE INDEX idx_search_documents_package_path_id ON search_documents(package_path_id);

END;
