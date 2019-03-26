-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE IF EXISTS imports;

CREATE TABLE dependencies (
	module_path TEXT NOT NULL,
	version TEXT NOT NULL,
	dependency_path TEXT NOT NULL,
	dependency_version TEXT NOT NULL,
	PRIMARY KEY (module_path, version),
	FOREIGN KEY (dependency_path, dependency_version) REFERENCES versions(module_path, version),
	FOREIGN KEY (module_path, version) REFERENCES versions(module_path, version)
);

END;
