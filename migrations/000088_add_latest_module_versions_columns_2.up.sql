-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE latest_module_versions
      ADD COLUMN series_path TEXT,
      ADD COLUMN deprecated BOOLEAN;

CREATE INDEX idx_latest_module_versions_series_path ON latest_module_versions(series_path);

END;
