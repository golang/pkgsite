-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents ALTER COLUMN name_tokens SET NOT NULL;
ALTER TABLE documents ALTER COLUMN path_tokens SET NOT NULL;

END;
