-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_package_symbols_module_path_id;

ALTER TABLE package_symbols
	ALTER COLUMN package_path_id TYPE BIGINT,
	ALTER COLUMN module_path_id TYPE BIGINT;

CREATE INDEX idx_package_symbols_module_path_id ON package_symbols(module_path_id);

END;
