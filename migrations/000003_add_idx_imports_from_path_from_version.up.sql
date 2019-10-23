-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_imports_from_path_from_version ON imports USING btree (from_path, from_version);
COMMENT ON INDEX idx_imports_from_path_from_version IS
'INDEX idx_imports_from_path_from_version is used to improve performance of the imports tab.';

END;
