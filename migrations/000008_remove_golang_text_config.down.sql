-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TEXT SEARCH CONFIGURATION golang (COPY = pg_catalog.english);

CREATE TEXT SEARCH DICTIONARY simple_english (
    TEMPLATE = pg_catalog.simple,
    STOPWORDS = english
);

ALTER TEXT SEARCH CONFIGURATION golang
    ALTER MAPPING FOR asciiword, asciihword, hword_asciipart, numword
    WITH simple_english;

ALTER TEXT SEARCH CONFIGURATION golang
    DROP MAPPING FOR url_path;


COMMENT ON TEXT SEARCH CONFIGURATION golang IS
'TEXT SEARCH CONFIGURATION golang is a custom search configuration used when creating tsvector for search.
The url_path token type is removed, so that "github.com/foo/bar@v1.2.3" is indexed only as the full URL string,
and not also"/foo/bar@v1.2.3".
The ASCII token types are set to a "simple_english" mapping, so that "plural" words like Postgres and NATS
will be indexed without stemming.
This idea came from the "Morphological and Exact Search" section here:
https://asp437.github.io/posts/flexible-fts.html.';

END;
