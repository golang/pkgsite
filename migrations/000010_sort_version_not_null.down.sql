-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE module_version_states ALTER COLUMN sort_version DROP NOT NULL;

ALTER TABLE versions ALTER COLUMN sort_version DROP NOT NULL;

END;
