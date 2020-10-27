-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_paths_path ON paths (path);
COMMENT ON INDEX idx_paths_path is
'INDEX idx_paths_path is used to get path information from a path.';

END;
