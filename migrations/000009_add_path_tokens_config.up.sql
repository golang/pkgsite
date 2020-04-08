-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TEXT SEARCH CONFIGURATION path_tokens (COPY = pg_catalog.english);

ALTER TEXT SEARCH CONFIGURATION path_tokens DROP MAPPING FOR hword_asciipart;

COMMENT ON TEXT SEARCH CONFIGURATION path_tokens IS
'TEXT SEARCH CONFIGURATION path_tokens is a custom search configuration used when creating a tsvector
from tokens that we generate from a path. The configuration ignores items that are part of a hyphenated
word, because our token generator already splits at hyphens.';

END;
