-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_documentation_symbols_package_symbol_id;
DROP INDEX idx_package_symbols_module_path_id;
DROP INDEX idx_package_symbols_package_path_id;
DROP INDEX idx_package_symbols_parent_symbol_name_id;
DROP INDEX idx_package_symbols_symbol_name_id;
DROP INDEX idx_package_symbols_section;
DROP INDEX idx_package_symbols_type;

END;
