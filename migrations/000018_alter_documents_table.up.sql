-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents DROP COLUMN name;
ALTER TABLE documents DROP COLUMN name_tokens;
ALTER TABLE documents DROP COLUMN module_synopsis_tokens;

ALTER TABLE documents RENAME COLUMN series_name TO package_path;
ALTER TABLE documents ALTER COLUMN package_name_tokens SET NOT NULL;

END;
