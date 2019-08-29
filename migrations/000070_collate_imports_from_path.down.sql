-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE imports_unique ALTER COLUMN to_path TYPE TEXT;
ALTER TABLE imports_unique ALTER COLUMN from_path TYPE TEXT;
ALTER TABLE imports_unique ALTER COLUMN from_module_path TYPE TEXT;

END;
