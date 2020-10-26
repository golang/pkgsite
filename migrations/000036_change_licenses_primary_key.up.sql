-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE licenses DROP CONSTRAINT licenses_pkey;
ALTER TABLE licenses ADD PRIMARY KEY (module_id, file_path);
DROP INDEX idx_licenses_module_id;

END;
