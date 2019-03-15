-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions DROP COLUMN readme;
ALTER TABLE versions DROP COLUMN license_name;
ALTER TABLE versions DROP COLUMN license_contents;
CREATE TABLE readmes (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	markdown TEXT NOT NULL,
	PRIMARY KEY (name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);

CREATE TABLE licenses (
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	contents TEXT NOT NULL,
	PRIMARY KEY (name, version),
	FOREIGN KEY (name, version) REFERENCES versions(name, version)
);
