-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documents ADD COLUMN tsv_search_tokens tsvector;
CREATE INDEX tsv_search_tokens_idx ON documents USING gin(tsv_search_tokens);

UPDATE documents SET tsv_search_tokens =
	name_tokens ||
	path_tokens ||
	synopsis_tokens ||
	readme_tokens;

END;
