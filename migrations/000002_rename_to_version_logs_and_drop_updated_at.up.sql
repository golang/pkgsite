-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE version_log RENAME TO version_logs;
ALTER TABLE version_logs DROP COLUMN updated_at;
