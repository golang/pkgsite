-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation DROP COLUMN id;
ALTER TABLE documentation DROP COLUMN new_goos;
ALTER TABLE documentation DROP COLUMN new_goarch;
DROP TRIGGER documentation_id_update ON documentation;
DROP FUNCTION update_documentation_id;

END;
