-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_version_map_module_id ON version_map (module_id);
CREATE INDEX idx_version_map_module_path ON version_map (module_path, resolved_version);

END;
