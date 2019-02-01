-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

DROP TABLE IF EXISTS
	dependencies,
	documents,
	licenses,
	modules,
	packages,
	readmes,
	series,
	version_log,
	versions;

DROP TYPE version_source;
