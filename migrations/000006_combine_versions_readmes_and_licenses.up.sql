-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions ADD COLUMN readme TEXT;
ALTER TABLE versions ADD COLUMN license_name TEXT;
ALTER TABLE versions ADD COLUMN license_contents TEXT;
DROP TABLE readmes, licenses;
