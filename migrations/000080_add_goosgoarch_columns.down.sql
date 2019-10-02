-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- Add columns to record the build context we used to parse the package.
-- See b/141852037.

BEGIN;

ALTER TABLE packages DROP COLUMN goos;
ALTER TABLE packages DROP COLUMN goarch;

END;
