-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

DROP TABLE
    modules,
    packages,
    imports,
    imports_unique,
    licenses,
    excluded_prefixes,
    module_version_states,
    search_documents,
    alternative_module_paths,
    experiments,
    package_version_states,
    version_map;

DROP FUNCTION
    hll_hash,
    hll_zeros,
    popular_search,
    popular_search_go_mod,
    trigger_modify_updated_at,
    trigger_modify_packages_tsv_parent_directories,
    trigger_modify_search_documents_tsv_parent_directories,
    to_tsvector_parent_directories;

DROP TYPE
    version_type,
    search_result;

DROP TEXT SEARCH CONFIGURATION golang;
