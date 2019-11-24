-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE module_version_states DROP COLUMN sort_version;

ALTER TABLE versions DROP COLUMN sort_version;

END;
