-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions ADD COLUMN version_type TEXT;
UPDATE versions SET
  version_type = packages.version_type
  FROM packages
  WHERE packages.module_path = versions.module_path;

END;
