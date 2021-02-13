-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_history RENAME signature TO synopsis;

COMMENT ON COLUMN symbol_history.synopsis IS
'COLUMN synopsis is a one-line summary for the symbol.';

END;
