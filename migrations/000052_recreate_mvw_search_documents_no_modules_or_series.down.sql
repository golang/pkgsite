-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP MATERIALIZED VIEW mvw_search_documents;

CREATE MATERIALIZED VIEW mvw_search_documents AS

SELECT
	p.path AS package_path,
	p.version,
	p.module_path,
	m.series_path,
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
INNER JOIN
	modules m
ON
	m.path = v.module_path
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
