-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TEXT SEARCH CONFIGURATION symbols (COPY = pg_catalog.simple);

-- No longer tokenize on dashes.
-- See https://www.postgresql.org/docs/11/textsearch-parsers.html
-- postgresql-beta1-lógico --> postgresql-beta1-lógico
-- postgresql is the hword_asciipart
-- beta1 is the hword_numpart
-- lógico is the hword_part
ALTER TEXT SEARCH CONFIGURATION symbols DROP MAPPING FOR hword_asciipart;
ALTER TEXT SEARCH CONFIGURATION symbols DROP MAPPING FOR hword_numpart;
ALTER TEXT SEARCH CONFIGURATION symbols DROP MAPPING FOR hword_part;

-- No longer tokenize on url_path, since this will be generated in the code.
-- For example:
-- github.com/foo/bar/baz -->  'github.com':2 'github.com/foo/bar/baz':1
--  (url,URL,github.com/foo/bar/baz,{simple},simple,{github.com/foo/bar/baz})
--  (host,Host,github.com,{simple},simple,{github.com})
--  (url_path,"URL path",/foo/bar/baz,{},,) (/foo/bar/baz is the url_part that is now dropped)
ALTER TEXT SEARCH CONFIGURATION symbols DROP MAPPING FOR url_path;

COMMENT ON TEXT SEARCH CONFIGURATION symbols IS
'TEXT SEARCH CONFIGURATION symbols is a custom search configuration used for symbol search. The configuration ignores items that are part of a hyphenated word and url_parts. These are handled in the code.';

END;
