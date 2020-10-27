-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE alternative_module_paths (
    alternative text NOT NULL,
    canonical text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    UNIQUE(alternative, canonical)
);
COMMENT ON TABLE alternative_module_paths IS
'TABLE alternative_module_paths contains module_paths that are known to have (1) a vanity import path, such as github.com/rsc/quote vs rsc.io/quote (2) a mismatch between the module path in the go.mod and repository, such as in the case of forks, or (3) a case insensitive spelling, such as in the case of github.com/sirupsen/logrus vs github.com/Sirupsen/logrus. It is used to filter out modules with the alternative path from the discovery site dataset.';
COMMENT ON COLUMN alternative_module_paths.alternative IS
'COLUMN alternative contains the path prefix of packages that should be filtered out from the discovery site search results. For example, github.com/google/go-cloud is the alternative prefix for all packages in the modules gocloud.dev and github.com/google/go-cloud.';
COMMENT ON COLUMN alternative_module_paths.canonical IS
'COLUMN canonical contains the module path that can be found in the go.mod file of a package. For example, gocloud.dev is the canonical prefix for all packages in gocloud.dev and github.com/google/go-cloud.';

END;
