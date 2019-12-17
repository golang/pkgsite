-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions
      DROP COLUMN major,
      DROP COLUMN minor,
      DROP COLUMN patch,
      DROP COLUMN prerelease;

END;
