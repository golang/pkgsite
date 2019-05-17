-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents
	DROP COLUMN IF EXISTS name_tokens,
	DROP COLUMN IF EXISTS path_tokens,
	DROP COLUMN IF EXISTS synopsis_tokens,
	DROP COLUMN IF EXISTS readme_tokens;

END;
