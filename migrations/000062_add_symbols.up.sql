-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TYPE symbol_section AS ENUM (
    'Constants',
    'Variables',
    'Functions',
    'Types'
);
COMMENT ON TYPE symbol_section IS
'ENUM symbol_section specifies the section that a symbol appears in on the documentation page.';

CREATE TYPE symbol_type AS ENUM (
    'Constant',
    'Variable',
    'Function',
    'Struct',
    'Interface',
    'Field',
    'Method'
);
COMMENT ON TYPE symbol_type IS
'ENUM symbol_type specifies the type of for a symbol in the symbol_history table.';

CREATE TYPE goos AS ENUM (
    'aix',
    'android',
    'darwin',
    'dragonfly',
    'freebsd',
    'illumos',
    'js',
    'linux',
    'netbsd',
    'openbsd',
    'plan9',
    'solaris',
    'windows',
    'all'
);
COMMENT ON TYPE goos IS
'ENUM goos specifies the execution operating system.';

CREATE TYPE goarch AS ENUM (
    '386',
    'amd64',
    'arm',
    'arm64',
    'mips',
    'mips64',
    'mips64le',
    'mipsle',
    'ppc64',
    'ppc64le',
    'riscv64',
    's390x',
    'wasm',
    'all'
);
COMMENT ON TYPE goarch IS
'ENUM goarch specifies the execution architecture.';

CREATE TABLE symbols (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    UNIQUE(name)
);

COMMENT ON TABLE symbols IS
'TABLE symbols contains all of the symbol names in the database. The name for a field or method expression is the <type-name>.<field-or-method-name>.';

CREATE TABLE symbol_history (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    v1path_id INTEGER NOT NULL,
    series_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    parent_symbol_id INTEGER NOT NULL,
    since_version TEXT NOT NULL,
    section symbol_section NOT NULL,
    signature TEXT NOT NULL,
    type symbol_type,
    goos goos NOT NULL,
    goarch goarch NOT NULL,
    UNIQUE(v1path_id, series_id, symbol_id, goos, goarch),

    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_symbol_id) REFERENCES symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (v1path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (series_id) REFERENCES paths(id) ON DELETE CASCADE
);
COMMENT ON TABLE symbol_history IS
'TABLE symbol_history documents the first version when a symbol was introduced in a package.';

COMMENT ON COLUMN symbol_history.parent_symbol_id IS
'COLUMN parent_symbol_id indicates the parent type for a symbol. If the symbol is the parent type, the parent_symbol_id will be equal to the symbol_id.';

END;
