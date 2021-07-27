-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Package name is used to rank <package>.<symbol> searches.
-- This value is only stored as a reference for uuid_package_name, which is
-- populated by a TRIGGER that reads from this column.
ALTER TABLE symbol_search_documents ADD COLUMN package_name TEXT;
-- UPDATE uuid_package_name on INSERT on symbol_search_documents.
-- The package_name should never change, so there is no need to change it on
-- UPDATE.
ALTER TABLE symbol_search_documents ADD COLUMN uuid_package_name UUID;
CREATE INDEX idx_symbol_search_documents_uuid_package_name ON symbol_search_documents (uuid_package_name);
CREATE FUNCTION trigger_modify_uuid_package_name() RETURNS TRIGGER AS $$
BEGIN
    NEW.uuid_package_name = uuid_generate_v5(uuid_nil(), NEW.package_name);
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;
CREATE TRIGGER set_uuid_package_name BEFORE INSERT ON symbol_search_documents
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_uuid_package_name();


-- Package path is used to rank <package>.<symbol> searches.
-- This value is only stored as a reference for uuid_package_path, which is
-- populated by a TRIGGER that reads from this column.
ALTER TABLE symbol_search_documents ADD COLUMN package_path TEXT;
-- UPDATE uuid_package_path on INSERT on symbol_search_documents.
-- The package_path should never change, so there is no need to change it on
-- UPDATE.
ALTER TABLE symbol_search_documents ADD COLUMN uuid_package_path UUID;
CREATE INDEX idx_symbol_search_documents_uuid_package_path ON symbol_search_documents (uuid_package_path);
CREATE FUNCTION trigger_modify_uuid_package_path() RETURNS TRIGGER AS $$
BEGIN
    NEW.uuid_package_path = uuid_generate_v5(uuid_nil(), NEW.package_path);
    RETURN NEW;
END
$$ LANGUAGE PLPGSQL;
CREATE TRIGGER set_uuid_package_path BEFORE INSERT ON symbol_search_documents
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_uuid_package_path();

END;
