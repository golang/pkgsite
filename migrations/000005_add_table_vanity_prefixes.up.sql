-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE vanity_prefixes (
    canonical TEXT NOT NULL,
    alternative TEXT NOT NULL,
    created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (canonical),
    UNIQUE(alternative)
);
COMMENT ON TABLE vanity_prefixes IS
'TABLE vanity_prefixes contains path prefixes that are known to be hosted other an alias name. (For example gocloud.dev can also be fetched from the module proxy as github.com/google/go-cloud.) It is used to filter out packages whose import paths begin with the alternative prefix from search results.';
COMMENT ON COLUMN vanity_prefixes.canonical IS
'COLUMN canonical contains the path prefix that can be found in the go.mod file of a package. For example, gocloud.dev is the canonical prefix for all packages in the modules gocloud.dev and github.com/google/go-cloud.';
COMMENT ON COLUMN vanity_prefixes.alternative IS
'COLUMN alternative contains the path prefix of packages that should be filtered out from the discovery site search results. For example, github.com/google/go-cloud is the alternative prefix for all packages in the modules gocloud.dev and github.com/google/go-cloud.';

END;
