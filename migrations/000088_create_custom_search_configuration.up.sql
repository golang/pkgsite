-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TEXT SEARCH CONFIGURATION public.golang (COPY = pg_catalog.english);

-- The url_path token type is remove, so that 'github.com/foo/bar@v1.2.3' is
-- indexed only as the full URL string, and not also '/foo/bar@v1.2.3'
ALTER TEXT SEARCH CONFIGURATION public.golang DROP MAPPING FOR url_path;

-- The asciiword token type is set to a 'simple,english_stem' mapping, so that
-- "plural" words will be indexed without stemming. We will need to update the
-- code to use both the simple and english configs for search and rank results
-- for simple higher than the english. This idea came from the "Morphological
-- and Exact Search" section here: https://asp437.github.io/posts/flexible-fts.html
ALTER TEXT SEARCH CONFIGURATION public.golang ALTER MAPPING FOR asciiword WITH simple,english_stem;

END;
