-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX package_symbols_package_path_id_module_path_id_section_synopsis_key;

CREATE UNIQUE INDEX package_symbols_package_path_id_module_path_id_section_synopsis_key
    ON package_symbols(package_path_id, module_path_id, section, synopsis);

END;
