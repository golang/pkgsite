-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation RENAME TO old_documentation;
ALTER TABLE new_documentation RENAME TO documentation;

ALTER INDEX documentation_pkey RENAME TO old_documentation_pkey;
ALTER TABLE old_documentation RENAME CONSTRAINT
    documentation_path_id_fkey TO old_documentation_unit_id_fkey;

ALTER INDEX new_documentation_pkey RENAME TO documentation_pkey;
ALTER INDEX new_documentation_unit_id_goos_goarch_key
    RENAME TO documentation_unit_id_goos_goarch_key;
ALTER TABLE documentation RENAME CONSTRAINT
    new_documentation_unit_id_fkey TO documentation_unit_id_fkey;

END;
