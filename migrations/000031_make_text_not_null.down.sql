-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions
  ALTER COLUMN readme DROP NOT NULL,
  ALTER COLUMN readme DROP DEFAULT,
  ALTER COLUMN prerelease DROP NOT NULL,
  ALTER COLUMN prerelease DROP DEFAULT,
  ALTER COLUMN build DROP NOT NULL,
  ALTER COLUMN build DROP DEFAULT,
  ALTER COLUMN readme_file_path DROP NOT NULL,
  ALTER COLUMN readme_file_path DROP DEFAULT,
  ALTER COLUMN readme_contents DROP NOT NULL,
  ALTER COLUMN readme_contents DROP DEFAULT,
  ALTER COLUMN version_type DROP NOT NULL,
  ALTER COLUMN version_type DROP DEFAULT;
ALTER TABLE packages
  ALTER COLUMN prerelease DROP NOT NULL,
  ALTER COLUMN prerelease DROP DEFAULT,
  ALTER COLUMN suffix DROP NOT NULL,
  ALTER COLUMN suffix DROP DEFAULT;

END;
