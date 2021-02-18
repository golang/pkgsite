-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation DROP CONSTRAINT documentation_pkey;
DROP INDEX documentation_unit_id_goos_goarch_key;
ALTER TABLE documentation ADD PRIMARY KEY (unit_id, goos, goarch);
ALTER TABLE documentation DROP COLUMN id;

END;
