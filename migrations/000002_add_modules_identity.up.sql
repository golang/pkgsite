-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER table modules ADD COLUMN id integer GENERATED ALWAYS AS IDENTITY UNIQUE;

END;
