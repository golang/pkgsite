-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE new_documentation (
    id bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    unit_id INTEGER NOT NULL,
    goos goos NOT NULL,
    goarch goarch NOT NULL,
    synopsis text NOT NULL,
    source bytea,
    UNIQUE (unit_id, goos, goarch),
    FOREIGN KEY (unit_id) REFERENCES units(id) ON DELETE CASCADE
);
COMMENT ON TABLE new_documentation
    IS 'TABLE documentation contains documentation for packages in the database.';
COMMENT ON COLUMN new_documentation.source
    IS 'COLUMN source contains the encoded ast.Files for the package.';

ALTER TABLE documentation_symbols DROP COLUMN id;
ALTER TABLE documentation_symbols RENAME COLUMN id_bigint TO id;
ALTER TABLE documentation_symbols ADD PRIMARY KEY (id);
ALTER TABLE documentation_symbols ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
ALTER TABLE documentation_symbols
    ADD CONSTRAINT documentation_symbols_documentation_id_fkey
    FOREIGN KEY (documentation_id)
    REFERENCES new_documentation(id) ON DELETE CASCADE;

ALTER TABLE package_symbols DROP COLUMN id CASCADE;
ALTER TABLE package_symbols RENAME COLUMN id_bigint TO id;
ALTER TABLE package_symbols ADD PRIMARY KEY (id);
ALTER TABLE package_symbols ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
ALTER TABLE documentation_symbols
    ADD CONSTRAINT documentation_symbols_package_id_fkey
    FOREIGN KEY (package_symbol_id)
    REFERENCES package_symbols(id) ON DELETE CASCADE;

END;
