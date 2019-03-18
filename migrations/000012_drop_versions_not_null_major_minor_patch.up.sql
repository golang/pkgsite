-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE versions ALTER COLUMN major DROP NOT NULL;
ALTER TABLE versions ALTER COLUMN minor DROP NOT NULL;
ALTER TABLE versions ALTER COLUMN patch DROP NOT NULL;
