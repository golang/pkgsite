-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX package_symbols_package_path_id_module_path_id_symbol_name_id_s;
CREATE UNIQUE INDEX package_symbols_package_path_id_module_path_id_symbol_name_id_parent_symbol_name_id_synopsis_key
    ON package_symbols(
        package_path_id,
        module_path_id,
        symbol_name_id,
        parent_symbol_name_id,
        uuid_generate_v5(uuid_nil(),
        synopsis));

END;
