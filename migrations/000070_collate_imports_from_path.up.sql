-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- Add collation to the columns of imports_unique. Without it, paths sort oddly.
-- See b/140185625.

BEGIN;

ALTER TABLE imports_unique ALTER COLUMN to_path TYPE TEXT COLLATE "C";
ALTER TABLE imports_unique ALTER COLUMN from_path TYPE TEXT COLLATE "C";
ALTER TABLE imports_unique ALTER COLUMN from_module_path TYPE TEXT COLLATE "C";

END;
