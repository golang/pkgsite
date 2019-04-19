-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- vw_search_results is a helper view that contains data needed to generate
-- search results.
CREATE OR REPLACE VIEW vw_search_results AS
	-- imported_by is the total number of imported_by of each package import
	-- path, at any version.
	WITH imported_by AS (
		SELECT
			imports.to_path AS package_path,
			COUNT(*) AS num_imported_by
		FROM (
			SELECT DISTINCT ON(from_path, to_path)
			from_path, to_path
			FROM imports
		) imports
		GROUP BY imports.to_path
	),
	-- latest_packages contains information for the latest version of each
	-- package.
	latest_packages AS (
		SELECT
			DISTINCT ON (vlp.path) vlp.path AS package_path,
			vlp.module_path,
			v.version,
			v.commit_time,
			vlp.license_types,
			vlp.license_paths,
			vlp.name,
			vlp.synopsis
		FROM
			vw_licensed_packages vlp
		INNER JOIN
			versions v
		ON
			v.module_path = vlp.module_path
			AND v.version = vlp.version
		ORDER BY
			vlp.path,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC
	)

	SELECT
		p.package_path,
		p.version,
		COALESCE(i.num_imported_by, 0) AS num_imported_by,
		p.module_path,
		p.commit_time,
		p.license_types,
		p.license_paths,
		p.name,
		p.synopsis,
		d.name_tokens,
		d.path_tokens,
		d.synopsis_tokens,
		d.readme_tokens
	FROM
		documents d
	INNER JOIN
		latest_packages p
	ON
		d.package_path = p.package_path
		AND d.version = p.version
	LEFT JOIN
		imported_by i
	ON
		i.package_path = p.package_path;

END;
