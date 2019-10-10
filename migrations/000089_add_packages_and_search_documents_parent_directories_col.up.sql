-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE packages ADD COLUMN tsv_parent_directories tsvector;
ALTER TABLE search_documents ADD COLUMN tsv_parent_directories tsvector;

CREATE INDEX idx_search_documents_tsv_parent_directories ON search_documents USING gin(tsv_parent_directories);
CREATE INDEX idx_packages_tsv_parent_directories ON packages USING gin(tsv_parent_directories);

-- Compute all directories that exist between modulePath and pkgPath, inclusive
-- of both modulePath and pkgPath. Return the result as a tsvector.
CREATE OR REPLACE FUNCTION to_tsvector_parent_directories(package_path TEXT, module_path TEXT)
RETURNS TSVECTOR AS $$
  DECLARE
    sub_path TEXT;
    parent_directories TEXT := module_path;
    sub_directories TEXT[][];
    current_directory TEXT := module_path;
    tsv_parent_directories TSVECTOR := module_path::tsvector;
  BEGIN
    IF package_path = module_path THEN
      RETURN tsv_parent_directories;
    END IF;

    sub_path := replace(package_path, module_path || '/', '');
    sub_directories := regexp_split_to_array(sub_path, '/');
    FOR i IN 1..cardinality(sub_directories) LOOP
      current_directory := current_directory || '/' || sub_directories[i];
      parent_directories = parent_directories || ' ' || current_directory;
    END LOOP;

    RETURN parent_directories::tsvector;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_modify_packages_tsv_parent_directories() RETURNS TRIGGER AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.path, NEW.module_path);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION trigger_modify_search_documents_tsv_parent_directories() RETURNS TRIGGER AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.package_path, NEW.module_path);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_tsv_parent_directories
BEFORE INSERT ON packages
FOR EACH ROW
EXECUTE PROCEDURE trigger_modify_packages_tsv_parent_directories();

CREATE TRIGGER set_tsv_parent_directories
BEFORE INSERT ON search_documents
FOR EACH ROW
EXECUTE PROCEDURE trigger_modify_search_documents_tsv_parent_directories();

END;
