-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE packages (
    path text NOT NULL,
    module_path text NOT NULL,
    version text NOT NULL,
    commit_time timestamp with time zone NOT NULL,
    name text NOT NULL,
    synopsis text,
    license_types text[],
    license_paths text[],
    v1_path text NOT NULL,
    goos text NOT NULL,
    goarch text NOT NULL,
    redistributable boolean DEFAULT false NOT NULL,
    documentation text,
    tsv_parent_directories tsvector,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (path, module_path, version),
    FOREIGN KEY (module_path, version) REFERENCES modules(module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE packages IS
'TABLE packages contains packages in a specific module version.';
COMMENT ON COLUMN packages.commit_time IS
'commit_time is the same as verions.commit_time. It is added here so that we can reduce the number of joins in our queries.';
COMMENT ON COLUMN packages.tsv_parent_directories IS
'tsv_parent_directories should always be NOT NULL, but it is populated by a trigger, so it will be initially NULL on insert.';

CREATE INDEX idx_packages_v1_path ON packages (v1_path);
COMMENT ON INDEX idx_packages_v1_path IS
'INDEX idx_packages_v1_path is used to get all of the packages in a series.';

CREATE INDEX idx_packages_module_path_text_pattern_ops ON packages (module_path text_pattern_ops);
COMMENT ON INDEX idx_packages_module_path_text_pattern_ops IS
'INDEX idx_packages_module_path_text_pattern_ops is used to improve performance of LIKE statements for module_path. It is used to fetch directories matching a given module_path prefix.';

CREATE INDEX idx_packages_path_text_pattern_ops ON packages (path text_pattern_ops);

END;
