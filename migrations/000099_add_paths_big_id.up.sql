-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE paths ADD COLUMN big_id bigint UNIQUE;

CREATE FUNCTION set_big_id() RETURNS TRIGGER AS $$
BEGIN
    -- Update big_id with the same value used for id.
    NEW.big_id = NEW.id;
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;

CREATE TRIGGER set_paths_big_id
BEFORE INSERT ON paths
FOR EACH ROW EXECUTE FUNCTION set_big_id();

END;
