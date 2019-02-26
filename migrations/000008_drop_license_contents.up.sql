-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions RENAME COLUMN license_name TO license;
ALTER TABLE versions DROP COLUMN license_contents;