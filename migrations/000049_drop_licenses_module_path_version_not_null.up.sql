-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE licenses
    ALTER COLUMN module_path DROP NOT NULL,
    ALTER COLUMN version DROP NOT NULL;

END;
