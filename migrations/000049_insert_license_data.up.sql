-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

UPDATE
	packages p
SET
	license_types = l.license_types,
	license_paths = l.license_paths
FROM (
        SELECT
		module_path,
		version,
		array_agg(l.type ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_types,
		array_agg(l.file_path ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_paths
        FROM licenses l
        GROUP BY 1,2
) l
WHERE
        p.module_path = l.module_path
        AND p.version = l.version;

END;
