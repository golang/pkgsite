-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE package_symbols
	ALTER COLUMN package_path_id TYPE INTEGER,
	ALTER COLUMN module_path_id TYPE INTEGER;

END;
