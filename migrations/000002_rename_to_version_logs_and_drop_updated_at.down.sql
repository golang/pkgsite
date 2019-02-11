-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE version_logs ADD COLUMN updated_at TIMESTAMP;
ALTER TABLE version_logs RENAME TO version_log;
