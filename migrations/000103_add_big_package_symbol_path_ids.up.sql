-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE package_symbols
	ADD COLUMN big_package_path_id BIGINT,
	ADD COLUMN big_module_path_id BIGINT;

CREATE FUNCTION set_big_path_ids() RETURNS TRIGGER AS $$
BEGIN
    NEW.big_package_path_id = NEW.package_path_id;
    NEW.big_module_path_id = NEW.module_path_id;
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;

CREATE TRIGGER set_package_symbols_big_package_path_id
BEFORE INSERT ON package_symbols
FOR EACH ROW EXECUTE FUNCTION set_big_path_ids();

END;
