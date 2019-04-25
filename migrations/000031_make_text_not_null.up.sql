-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

UPDATE versions SET readme='' WHERE readme IS NULL;
UPDATE versions SET prerelease='' WHERE prerelease IS NULL;
UPDATE versions SET build='' WHERE build IS NULL;
UPDATE versions SET readme_file_path='' WHERE readme_file_path IS NULL;
UPDATE versions SET readme_contents='' WHERE readme_contents IS NULL;
UPDATE versions SET version_type='' WHERE version_type IS NULL;
UPDATE packages SET prerelease='' WHERE prerelease IS NULL;
UPDATE packages set suffix='' WHERE suffix IS NULL;

ALTER TABLE versions
  ALTER COLUMN readme SET NOT NULL,
  ALTER COLUMN readme SET DEFAULT '',
  ALTER COLUMN prerelease SET NOT NULL,
  ALTER COLUMN prerelease SET DEFAULT '',
  ALTER COLUMN build SET NOT NULL,
  ALTER COLUMN build SET DEFAULT '',
  ALTER COLUMN readme_file_path SET NOT NULL,
  ALTER COLUMN readme_file_path SET DEFAULT '',
  ALTER COLUMN readme_contents SET NOT NULL,
  ALTER COLUMN readme_contents SET DEFAULT '',
  ALTER COLUMN version_type SET NOT NULL,
  ALTER COLUMN version_type SET DEFAULT '';
ALTER TABLE packages
  ALTER COLUMN prerelease SET NOT NULL,
  ALTER COLUMN prerelease SET DEFAULT '',
  ALTER COLUMN suffix SET NOT NULL,
  ALTER COLUMN suffix SET DEFAULT '';

END;
