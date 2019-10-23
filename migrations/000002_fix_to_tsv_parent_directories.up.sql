-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.
--
-- This schema migration is created from squashing all of our existing
-- migration files in
-- 
BEGIN;

CREATE OR REPLACE FUNCTION to_tsvector_parent_directories(package_path text, module_path text) RETURNS tsvector
    LANGUAGE plpgsql
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

    -- +2 because substr is one-based and we need to include the trailing slash
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

UPDATE packages SET tsv_parent_directories = to_tsvector_parent_directories(path, module_path);
UPDATE search_documents SET tsv_parent_directories = to_tsvector_parent_directories(package_path, module_path);

END;
