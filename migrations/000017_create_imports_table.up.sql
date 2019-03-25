-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE imports (
	from_path TEXT NOT NULL,
	from_module_path TEXT NOT NULL,
	from_version TEXT NOT NULL,
	to_path TEXT NOT NULL,
	to_name TEXT NOT NULL,
	transitive BOOL NOT NULL,
	PRIMARY KEY(from_path, from_version, to_path),
	FOREIGN KEY (from_path, from_module_path, from_version) REFERENCES packages(path, module_path, version)
);

DROP TABLE IF EXISTS dependencies;

END;
