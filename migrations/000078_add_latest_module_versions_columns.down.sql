-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE latest_module_versions DROP COLUMN status;
ALTER TABLE latest_module_versions DROP COLUMN updated_at;
DROP INDEX idx_latest_module_versions_status;

END;
