-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE latest_module_versions DROP COLUMN series_path;
ALTER TABLE latest_module_versions DROP COLUMN deprecated;
DROP INDEX idx_latest_module_versions_series_path;

END;
