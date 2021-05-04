-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_symbol_history_module_path_id ON symbol_history(module_path_id);
CREATE INDEX idx_symbol_history_since_version ON symbol_history(since_version);
CREATE INDEX idx_symbol_history_symbol_name_id ON symbol_history(symbol_name_id);
CREATE INDEX idx_symbol_history_parent_symbol_name_id ON symbol_history(parent_symbol_name_id);

END;
