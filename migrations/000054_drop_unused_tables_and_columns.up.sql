-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP VIEW vw_licensed_packages;
DROP TABLE version_logs;
DROP TABLE package_licenses;
DROP TABLE modules;
DROP TABLE series;

ALTER TABLE versions
	DROP COLUMN readme,
	DROP COLUMN synopsis,
	DROP COLUMN deleted;

ALTER TABLE packages
	DROP COLUMN major,
	DROP COLUMN minor,
	DROP COLUMN patch,
	DROP COLUMN prerelease,
	DROP COLUMN version_type;

ALTER TABLE documents
	DROP COLUMN deleted;

END;
