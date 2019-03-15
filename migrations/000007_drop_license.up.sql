-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions ALTER COLUMN license_name SET NOT NULL;
ALTER TABLE versions DROP COLUMN license;