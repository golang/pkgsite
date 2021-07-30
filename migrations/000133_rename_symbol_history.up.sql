-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_history RENAME TO old_symbol_history;
ALTER TABLE new_symbol_history RENAME TO symbol_history;

END;
