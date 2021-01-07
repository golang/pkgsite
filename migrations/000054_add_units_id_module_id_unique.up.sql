-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units ALTER COLUMN path_id SET NOT NULL;
ALTER TABLE units ADD CONSTRAINT units_path_id_module_id_key UNIQUE(path_id, module_id);
ALTER TABLE units ALTER COLUMN path DROP NOT NULL;

END;
