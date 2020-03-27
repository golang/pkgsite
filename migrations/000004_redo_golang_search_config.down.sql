-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TEXT SEARCH CONFIGURATION golang;

CREATE TEXT SEARCH CONFIGURATION golang (
    PARSER = pg_catalog."default" );

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR asciiword WITH simple, english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR word WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR numword WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR email WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR url WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR host WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR sfloat WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR version WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR hword_numpart WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR hword_part WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR hword_asciipart WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR numhword WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR asciihword WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR hword WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR file WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR "float" WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR "int" WITH simple;

ALTER TEXT SEARCH CONFIGURATION golang
    ADD MAPPING FOR uint WITH simple;

COMMENT ON TEXT SEARCH CONFIGURATION golang IS
'TEXT SEARCH CONFIGURATION golang is a custom search configuration used when creating tsvector for search. The url_path token type is remove, so that "github.com/foo/bar@v1.2.3" is indexed only as the full URL string, and not also"/foo/bar@v1.2.3". The asciiword token type is set to a "simple,english_stem" mapping, so that "plural" words will be indexed without stemming. This idea came from the "Morphological and Exact Search" section here: https://asp437.github.io/posts/flexible-fts.html.';


END;
