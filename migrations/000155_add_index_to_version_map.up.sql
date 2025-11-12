-- Copyright 2025 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_version_map_requested_version_module_path_resolved_version
ON version_map (requested_version, module_path, resolved_version);

END;
