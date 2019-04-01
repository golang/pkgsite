-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;
DROP TABLE IF EXISTS documents;

-- Documents stores tokens for package versions that used for search.
CREATE TABLE documents(
	created_at TIMESTAMP NOT NULL DEFAULT NOW(),
	package_path TEXT NOT NULL NOT NULL,
	module_path TEXT NOT NULL NOT NULL,
	series_path TEXT NOT NULL NOT NULL,
	package_suffix TEXT NOT NULL NOT NULL,

	version TEXT NOT NULL,

	deleted BOOL NOT NULL DEFAULT FALSE, 

	-- tsvector for the package name
	name_tokens TSVECTOR NOT NULL, -- weight 1.0

	-- tsvector for the package, module and series path
	path_tokens TSVECTOR NOT NULL, -- weight 1.0

	synopsis_tokens TSVECTOR, -- weight 0.4
	readme_tokens TSVECTOR, -- weight 0.2

	PRIMARY KEY (package_path, module_path, version),
	FOREIGN KEY (package_path, module_path, version) REFERENCES packages(path, module_path, version)
);

END;
