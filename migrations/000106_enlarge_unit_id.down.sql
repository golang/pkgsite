-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER SEQUENCE paths_id_seq AS INTEGER;

ALTER TABLE units ALTER COLUMN id TYPE INTEGER;


END;
