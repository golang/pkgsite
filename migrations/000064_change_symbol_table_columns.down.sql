-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_history RENAME module_path_id TO series_id;
ALTER TABLE symbol_history RENAME package_path_id TO v1path_id;

END;
