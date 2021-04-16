-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER INDEX documentation_pkey RENAME TO new_documentation_pkey;
ALTER INDEX documentation_unit_id_goos_goarch_key
    RENAME TO new_documentation_unit_id_goos_goarch_key;
ALTER TABLE documentation RENAME CONSTRAINT
    documentation_unit_id_fkey TO new_documentation_unit_id_fkey;

ALTER INDEX old_documentation_pkey RENAME TO documentation_pkey;
ALTER TABLE old_documentation RENAME CONSTRAINT
    old_documentation_unit_id_fkey TO documentation_path_id_fkey;

ALTER TABLE documentation RENAME TO new_documentation;
ALTER TABLE old_documentation RENAME TO documentation;

END;
