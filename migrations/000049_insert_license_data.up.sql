-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

UPDATE
	packages p
SET
	license_types = pl.license_types,
	license_paths = pl.license_paths
FROM (
        SELECT
		pl.module_path,
		pl.version,
		pl.package_path,
		array_agg(l.type ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_types,
		array_agg(l.file_path ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_paths
        FROM licenses l
	INNER JOIN package_licenses pl
        ON pl.module_path=l.module_path AND pl.version=l.version AND pl.file_path=l.file_path
        GROUP BY 1,2,3
) pl
WHERE
        p.module_path = pl.module_path
        AND p.version = pl.version
	AND p.path = pl.package_path;

END;
