-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_documentation_symbols_package_symbol_id ON documentation_symbols(package_symbol_id);
CREATE INDEX idx_package_symbols_module_path_id ON package_symbols(module_path_id);
CREATE INDEX idx_package_symbols_package_path_id ON package_symbols(package_path_id);
CREATE INDEX idx_package_symbols_parent_symbol_name_id ON package_symbols(parent_symbol_name_id);
CREATE INDEX idx_package_symbols_symbol_name_id ON package_symbols(symbol_name_id);
CREATE INDEX idx_package_symbols_section ON package_symbols(section);
CREATE INDEX idx_package_symbols_type ON package_symbols(type);

END;
