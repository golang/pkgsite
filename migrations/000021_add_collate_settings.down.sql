-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions ALTER COLUMN prerelease TYPE TEXT;
ALTER TABLE packages ALTER COLUMN path TYPE TEXT;
ALTER TABLE packages ALTER COLUMN module_path TYPE TEXT;

END;
