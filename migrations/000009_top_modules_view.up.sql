-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION all_redistributable(types text[]) RETURNS boolean AS $$
    SELECT COALESCE(types <@ ARRAY['AGPL-3.0', 'Apache-2.0', 'Artistic-2.0', 'BSD-2-Clause',
        'BSD-3-Clause', 'BSL-1.0', 'GPL2', 'GPL3', 'ISC', 'LGPL-2.1',
	'LGPL-3.0', 'MIT', 'MPL-2.0', 'Zlib'], false);
$$ LANGUAGE SQL;

COMMENT ON FUNCTION all_redistributable IS
'FUNCTION all_redistributable reports whether all types in the argument are redistributable license types.';


CREATE OR REPLACE VIEW vw_module_licenses AS
WITH top_modules AS (
    -- Get the most popular modules from search_documents.
    SELECT
        module_path,
        module_imported_by_count,
        redistributable,
        rank() OVER (ORDER BY module_imported_by_count desc) rank
    FROM  (
        SELECT module_path, max(imported_by_count) AS module_imported_by_count, redistributable
        FROM search_documents
        WHERE module_path != 'std'
        GROUP BY module_path, redistributable
        ORDER BY max(imported_by_count) DESC
    ) a
    WHERE module_imported_by_count > 10
), max_sort_versions AS (
    -- Find the max sort_version of each of those modules.
     SELECT v.module_path, MAX(v.sort_version) AS sort_version, t.module_imported_by_count, t.rank
     FROM versions v, top_modules t
     WHERE v.module_path = t.module_path
     GROUP BY v.module_path, t.module_imported_by_count, t.rank
), max_versions AS (
   -- Get versions from sort versions.
   SELECT v.module_path, v.version, s.module_imported_by_count, s.rank
   FROM versions v
   INNER JOIN max_sort_versions s
   USING (module_path, sort_version)
), top_level_licenses AS (
   -- Get licenses at the module top level.
    SELECT l.module_path, l.version, l.file_path, l.types, m.module_imported_by_count, m.rank, l.coverage
    FROM licenses l
    INNER JOIN max_versions m
    USING (module_path, version)
    WHERE position('/' in l.file_path) = 0
)
SELECT
    m.module_path,
    l.version,
    l.file_path,
    l.types,
    m.module_imported_by_count,
    m.rank,
    l.coverage,
    CASE
        WHEN l.module_path IS NULL THEN 'No license'
        WHEN NOT all_redistributable(l.types) THEN 'Unsupported license'
    END AS reason_not_redistributable
FROM max_versions m
LEFT JOIN top_level_licenses l
ON m.module_path = l.module_path;

COMMENT ON VIEW vw_module_licenses IS
'VIEW vm_module_licenses holds license information for the most popular modules.
(Those where the max imported-by count of any package in the module is over 10).
The modules are ranked by imported-by count.
The built-in rank function assigns the same rank to equal values.';

END;
