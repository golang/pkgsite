-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS units_path_id_module_id_key ON units(path_id, module_id);
