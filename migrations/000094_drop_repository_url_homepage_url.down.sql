-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions ADD COLUMN repository_url TEXT;
ALTER TABLE versions ADD COLUMN homepage_url TEXT;

END;
