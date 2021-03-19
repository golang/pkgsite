-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation
    ADD COLUMN id INTEGER,
    ADD COLUMN id_bigint BIGINT,
    ADD COLUMN new_goos goos,
    ADD COLUMN new_goarch goarch,
    ADD COLUMN zip bytea;

CREATE TRIGGER documentation_id_update
BEFORE INSERT OR UPDATE ON documentation
FOR EACH ROW EXECUTE PROCEDURE update_documentation_id();

END;
