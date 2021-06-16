-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation_symbols RENAME TO legacy_documentation_symbols;
ALTER INDEX idx_documentation_symbols_documentation_id RENAME TO
    idx_legacy_documentation_symbols_documentation_id;
ALTER INDEX idx_documentation_symbols_package_symbol_id RENAME TO
    idx_legacy_documentation_symbols_package_symbol_id;

CREATE TABLE documentation_symbols (
    id bigint NOT NULL PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    documentation_id bigint NOT NULL,
    package_symbol_id bigint NOT NULL,
    UNIQUE (documentation_id, package_symbol_id),
    FOREIGN KEY (documentation_id) REFERENCES documentation(id) ON DELETE CASCADE,
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE
);
COMMENT ON TABLE documentation_symbols IS 'TABLE documentation_symbols contains symbols for a given row in the documentation table.';
CREATE INDEX idx_documentation_symbols_documentation_id ON
    documentation_symbols USING btree (documentation_id);
CREATE INDEX idx_documentation_symbols_package_symbol_id ON
    documentation_symbols USING btree (package_symbol_id);

END;
