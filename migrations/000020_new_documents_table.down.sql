-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE documents;

CREATE TABLE documents (
	package_path TEXT NOT NULL,
	version TEXT NOT NULL,
	package_name_tokens TSVECTOR NOT NULL,
	package_synopsis_tokens TSVECTOR,
	readme_tokens TSVECTOR,
	created_at TIMESTAMP DEFAULT NOW(),
    	FOREIGN KEY (package_path) REFERENCES series(path)
);

END;
