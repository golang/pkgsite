-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE FUNCTION to_tsvector_parent_directories(package_path text, module_path text) RETURNS tsvector
    LANGUAGE plpgsql PARALLEL SAFE
    AS $$
  DECLARE
    current_directory TEXT;
    parent_directories TEXT;
    sub_path TEXT;
    sub_directories TEXT[][];
  BEGIN
    IF package_path = module_path THEN
      RETURN module_path::tsvector;
    END IF;

    IF module_path = 'std' THEN
      sub_path := package_path;
    ELSE
      sub_path := substr(package_path, length(module_path) + 2);
      current_directory := module_path;
      parent_directories := module_path;
    END IF;

    sub_directories := regexp_split_to_array(sub_path, '/');
    FOR i IN 1..cardinality(sub_directories) LOOP
      IF current_directory IS NULL THEN
	current_directory := sub_directories[i];
      ELSE
        current_directory := COALESCE(current_directory, '') || '/' || sub_directories[i];
      END IF;
      parent_directories = COALESCE(parent_directories, '') || ' ' || current_directory;
    END LOOP;
    RETURN parent_directories::tsvector;
END;
$$;
COMMENT ON FUNCTION to_tsvector_parent_directories IS
'FUNCTION to_tsvector_parent_directories computes all directories that exist between module_path and package_path, inclusive of both module_path and package_path. Return the result as a tsvector.';

CREATE FUNCTION trigger_modify_packages_tsv_parent_directories() RETURNS TRIGGER
    LANGUAGE plpgsql
    AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.path, NEW.module_path);
  RETURN NEW;
END;
$$;
COMMENT ON FUNCTION trigger_modify_packages_tsv_parent_directories IS
'FUNCTION trigger_modify_packages_tsv_parent_directories invokes FUNCTION to_tsvector_parent_directories and sets the value of tsv_parent_directories to the output.';

ALTER TABLE packages ADD COLUMN tsv_parent_directories tsvector;

CREATE TRIGGER set_tsv_parent_directories BEFORE INSERT ON packages
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_packages_tsv_parent_directories();
COMMENT ON TRIGGER set_tsv_parent_directories ON packages IS
'TRIGGER set_tsv_parent_directories sets the value of tsv_parent_directories to the output of FUNCTION trigger_modify_search_documents_tsv_parent_directories when a new row in inserted.';

ALTER TABLE search_documents ADD COLUMN tsv_parent_directories tsvector;

CREATE TRIGGER set_tsv_parent_directories BEFORE INSERT ON search_documents
	FOR EACH ROW EXECUTE PROCEDURE trigger_modify_search_documents_tsv_parent_directories();
COMMENT ON TRIGGER set_tsv_parent_directories ON search_documents IS
'TRIGGER set_tsv_parent_directories sets the value of tsv_parent_directories to the output of FUNCTION trigger_modify_search_documents_tsv_parent_directories when a new row in inserted.';

END;
