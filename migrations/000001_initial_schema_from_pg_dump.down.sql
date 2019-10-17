-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

DROP TABLE
	versions,
	packages,
	imports,
	imports_unique,
	licenses,
	excluded_prefixes,
	module_version_states,
	search_documents;

DROP FUNCTION
	hll_hash,
	hll_zeros,
	popular_search,
	trigger_modify_updated_at,
	to_tsvector_parent_directories;

DROP TYPE
	version_type,
	search_result;

DROP TEXT SEARCH CONFIGURATION golang;
