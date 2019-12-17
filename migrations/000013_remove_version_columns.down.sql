-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions
    ADD COLUMN major integer,
    ADD COLUMN minor integer,
    ADD COLUMN patch integer,
    ADD COLUMN prerelease text;

-- Write your migration here.

END;
