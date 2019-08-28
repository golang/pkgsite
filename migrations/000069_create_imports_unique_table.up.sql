-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE imports_unique (
	to_path TEXT NOT NULL,
	from_path TEXT NOT NULL,
	from_module_path TEXT NOT NULL,
	PRIMARY KEY (to_path, from_path, from_module_path)
);

END;
