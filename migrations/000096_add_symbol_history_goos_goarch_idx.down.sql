-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_symbol_history_module_sort_version;
DROP INDEX idx_symbol_history_goos;
DROP INDEX idx_symbol_history_goarch;

END;
