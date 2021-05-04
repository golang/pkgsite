-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_symbol_history_module_path_id;
DROP INDEX idx_symbol_history_since_version;
DROP INDEX idx_symbol_history_symbol_name_id;
DROP INDEX idx_symbol_history_parent_symbol_name_id;

END;
