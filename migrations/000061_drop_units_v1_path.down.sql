-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units ADD COLUMN v1_path TEXT;
CREATE INDEX units_v1_path_key ON units(v1_path);

END;
