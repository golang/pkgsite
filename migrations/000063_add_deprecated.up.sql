-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules ADD COLUMN deprecated_comment TEXT;

COMMENT ON COLUMN modules.deprecated_comment IS
'COLUMN deprecated_comment holds the "Deprecated" comment from the go.mod file, if any.';

END;
