-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions ALTER COLUMN major SET NOT NULL;
ALTER TABLE versions ALTER COLUMN minor SET NOT NULL;
ALTER TABLE versions ALTER COLUMN patch SET NOT NULL;
