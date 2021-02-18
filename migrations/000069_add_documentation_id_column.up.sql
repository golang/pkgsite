-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation DROP CONSTRAINT documentation_pkey;
ALTER TABLE documentation ADD COLUMN id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY;
CREATE UNIQUE INDEX documentation_unit_id_goos_goarch_key ON documentation (unit_id, goos, goarch);

END;
