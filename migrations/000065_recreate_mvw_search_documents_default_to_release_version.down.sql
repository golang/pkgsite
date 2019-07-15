-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Different to the documentation of CREATE VIEW
-- (https://www.postgresql.org/docs/11/sql-createview.html),
-- the documentation of CREATE MATERIALIZED VIEW
-- (https://www.postgresql.org/docs/11/sql-creatematerializedview.html)
-- does NOT mention the REPLACE keyword. There seems to be no shortcut aside
-- from dropping all dependent objects and rebuilding each one.
DROP MATERIALIZED VIEW mvw_search_documents;

-- mvw_search_documents is a materialize view that contains data needed to
-- generate search results.
CREATE MATERIALIZED VIEW mvw_search_documents AS

SELECT
	p.path AS package_path,
	p.version,
	p.module_path,
	COALESCE(i.num_imported_by, 0) AS num_imported_by,
	p.name,
	v.commit_time,
	p.synopsis,
	p.license_types,
	d.tsv_search_tokens
FROM
	packages p
INNER JOIN
	documents d
ON
	d.module_path = p.module_path
	AND d.version = p.version
	AND d.package_path = p.path
INNER JOIN (
	SELECT
		DISTINCT ON (module_path) module_path,
		version,
		readme_contents,
		commit_time
	FROM
		versions
	ORDER BY
		module_path,
		major DESC,
		minor DESC,
		patch DESC,
		prerelease DESC
) v
ON
	v.module_path = p.module_path
	AND v.version = p.version
LEFT JOIN (
	SELECT
		to_path,
		COUNT(DISTINCT(from_path)) AS num_imported_by
	FROM
		imports
	WHERE
		strpos(to_path, from_module_path) = 0
	GROUP BY 1
) i
ON
	i.to_path = p.path
WHERE
	p.path NOT LIKE '%/internal%'
;

CREATE INDEX mvw_search_documents_tsv_search_tokens_idx ON mvw_search_documents USING gin(tsv_search_tokens);
CREATE UNIQUE INDEX mvw_search_documents_package_path_module_path_version_unique_idx ON mvw_search_documents(package_path, module_path, version);

END;
