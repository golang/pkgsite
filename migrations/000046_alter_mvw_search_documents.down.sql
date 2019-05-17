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
	pl.license_types,
	(
		SETWEIGHT(TO_TSVECTOR(p.name), 'A') ||
		SETWEIGHT(TO_TSVECTOR(p.path), 'A') ||
		SETWEIGHT(TO_TSVECTOR(p.module_path), 'A') ||
		SETWEIGHT(TO_TSVECTOR(m.series_path), 'A') ||
		SETWEIGHT(TO_TSVECTOR(p.synopsis), 'B') ||
		SETWEIGHT(TO_TSVECTOR(v.readme_contents), 'C')
	) AS tsv_search_tokens
FROM
	packages p
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
		pl.module_path,
		pl.version,
		pl.package_path,
		array_agg(l.type) FILTER ( WHERE l.version IS NOT NULL) AS license_types
	FROM
		package_licenses pl
	LEFT JOIN
		licenses l
	ON
		pl.module_path = l.module_path
		AND pl.version = l.version
		AND pl.file_path = l.file_path
	GROUP BY
		pl.module_path,
		pl.version,
		pl.package_path
) pl
ON
	pl.module_path = p.module_path
	AND pl.version = p.version
	AND pl.package_path = p.path
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
