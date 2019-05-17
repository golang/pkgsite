-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents
	ADD COLUMN name_tokens TSVECTOR,
	ADD COLUMN path_tokens TSVECTOR,
	ADD COLUMN synopsis_tokens TSVECTOR,
	ADD COLUMN readme_tokens TSVECTOR;

END;
