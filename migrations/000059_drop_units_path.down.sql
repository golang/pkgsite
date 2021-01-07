-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units ADD COLUMN path TEXT;
ALTER TABLE units ADD CONSTRAINT units_path_module_id_key UNIQUE(path, module_id);

END;
