-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE packages DROP COLUMN major;
ALTER TABLE packages DROP COLUMN minor;
ALTER TABLE packages DROP COLUMN patch;
ALTER TABLE packages DROP COLUMN prerelease;