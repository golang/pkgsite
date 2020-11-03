-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE paths ADD CONSTRAINT paths_path_key UNIQUE (path);
CREATE INDEX idx_paths_path_id ON paths(path, id);

END;
