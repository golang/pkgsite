-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION to_tsvector_parent_directories(package_path text, module_path text) RETURNS tsvector
    LANGUAGE plpgsql
    AS $$
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
    -- +2 because substr is one-based and we need to include the trailing slash
    sub_path := substr(package_path, length(module_path) + 2);
    sub_directories := regexp_split_to_array(sub_path, '/');
    FOR i IN 1..cardinality(sub_directories) LOOP
      current_directory := current_directory || '/' || sub_directories[i];
      parent_directories = parent_directories || ' ' || current_directory;
    END LOOP;
    RETURN parent_directories::tsvector;
END;
$$;

END;
