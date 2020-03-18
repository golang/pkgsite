-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_imported_by_count_gt_8 ON search_documents USING btree (package_path) WHERE (imported_by_count > 8);
CREATE INDEX idx_imported_by_count_gt_50 ON search_documents USING btree (package_path) WHERE (imported_by_count > 50);

CREATE FUNCTION all_redistributable(types text[]) RETURNS boolean
    LANGUAGE sql
    AS $$
    SELECT COALESCE(types <@ ARRAY['AGPL-3.0', 'Apache-2.0', 'Artistic-2.0', 'BSD-2-Clause',
        'BSD-3-Clause', 'BSL-1.0', 'GPL2', 'GPL3', 'ISC', 'LGPL-2.1',
	'LGPL-3.0', 'MIT', 'MPL-2.0', 'Zlib'], false);
$$;

CREATE VIEW vw_module_licenses AS
 WITH top_modules AS (
         SELECT a.module_path,
            a.module_imported_by_count,
            a.redistributable,
            rank() OVER (ORDER BY a.module_imported_by_count DESC) AS rank
           FROM ( SELECT search_documents.module_path,
                    max(search_documents.imported_by_count) AS module_imported_by_count,
                    search_documents.redistributable
                   FROM search_documents
                  WHERE (search_documents.module_path <> 'std'::text)
                  GROUP BY search_documents.module_path, search_documents.redistributable
                  ORDER BY (max(search_documents.imported_by_count)) DESC) a
          WHERE (a.module_imported_by_count > 10)
        ), max_sort_versions AS (
         SELECT v.module_path,
            max(v.sort_version) AS sort_version,
            t.module_imported_by_count,
            t.rank
           FROM modules v,
            top_modules t
          WHERE (v.module_path = t.module_path)
          GROUP BY v.module_path, t.module_imported_by_count, t.rank
        ), max_versions AS (
         SELECT v.module_path,
            v.version,
            s.module_imported_by_count,
            s.rank
           FROM (modules v
             JOIN max_sort_versions s USING (module_path, sort_version))
        ), top_level_licenses AS (
         SELECT l_1.module_path,
            l_1.version,
            l_1.file_path,
            l_1.types,
            m_1.module_imported_by_count,
            m_1.rank,
            l_1.coverage
           FROM (licenses l_1
             JOIN max_versions m_1 USING (module_path, version))
          WHERE ("position"(l_1.file_path, '/'::text) = 0)
        )
 SELECT m.module_path,
    l.version,
    l.file_path,
    l.types,
    m.module_imported_by_count,
    m.rank,
    l.coverage,
        CASE
            WHEN (l.module_path IS NULL) THEN 'No license'::text
            WHEN (NOT all_redistributable(l.types)) THEN 'Unsupported license'::text
            ELSE NULL::text
        END AS reason_not_redistributable
   FROM (max_versions m
     LEFT JOIN top_level_licenses l ON ((m.module_path = l.module_path)));

END;
