-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TRIGGER set_package_symbols_big_package_path_id ON package_symbols;

ALTER TABLE package_symbols
	DROP COLUMN big_package_path_id,
	DROP COLUMN big_module_path_id;

END;
