-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents ADD COLUMN name TEXT NOT NULL;
ALTER TABLE documents ADD COLUMN name_tokens TSVECTOR;
ALTER TABLE documents ADD COLUMN module_synopsis_tokens TSVECTOR;

ALTER TABLE documents RENAME COLUMN package_path TO series_name;
ALTER TABLE documents ALTER COLUMN package_name_tokens DROP NOT NULL;

END;
