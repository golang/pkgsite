-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions DROP CONSTRAINT unique_semver;
ALTER TABLE versions DROP COLUMN major;
ALTER TABLE versions DROP COLUMN minor;
ALTER TABLE versions DROP COLUMN patch;
ALTER TABLE versions DROP COLUMN prerelease;
ALTER TABLE versions DROP COLUMN build;