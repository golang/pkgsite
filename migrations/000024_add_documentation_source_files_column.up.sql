-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation ADD COLUMN source BYTEA;
ALTER TABLE documentation ADD COLUMN zip BYTEA;

COMMENT ON COLUMN documentation.source IS
'COLUMN source contains the uncompressed zip of the source files for the package.';
COMMENT ON COLUMN documentation.zip IS
'COLUMN zip contains the compressed zip of the source files for the package.';

END;
