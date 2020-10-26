-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_licenses_module_id ON licenses (module_id);
ALTER TABLE licenses DROP CONSTRAINT licenses_pkey;
ALTER TABLE licenses ADD PRIMARY KEY (module_path, version, file_path);

END;
