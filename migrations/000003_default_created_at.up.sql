-- Copyright 2009 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

ALTER TABLE series ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE modules ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE versions ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE version_logs ALTER COLUMN created_at SET DEFAULT current_timestamp;
ALTER TABLE documents ADD COLUMN created_at TIMESTAMP DEFAULT current_timestamp;
