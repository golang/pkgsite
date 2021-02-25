-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation ADD COLUMN id INTEGER;
ALTER TABLE documentation ADD COLUMN new_goos goos;
ALTER TABLE documentation ADD COLUMN new_goarch goarch;

CREATE SEQUENCE sequence_documentation_id START 1;
CREATE OR REPLACE FUNCTION update_documentation_id() RETURNS TRIGGER AS $BODY$
BEGIN
    NEW.id=nextval('sequence_documentation_id');
    RETURN NEW;
END
$BODY$ LANGUAGE PLPGSQL;
ALTER SEQUENCE sequence_documentation_id OWNED BY documentation.id;

CREATE TRIGGER documentation_id_update
BEFORE INSERT OR UPDATE ON documentation
FOR EACH ROW EXECUTE PROCEDURE update_documentation_id();

END;
