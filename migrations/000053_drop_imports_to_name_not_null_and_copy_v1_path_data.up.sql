-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Set data for packages.v1_path
UPDATE packages p
SET v1_path = p2.v1_path
FROM (
        SELECT
                p.path,
                p.module_path,
                p.version,
		CASE WHEN p.suffix = '.' OR p.suffix = '' THEN series_path
		ELSE concat(m.series_path, '/', p.suffix) END
                AS v1_path
	FROM packages p
	INNER JOIN modules m
	ON m.path=p.module_path
) p2
WHERE
        p.path = p2.path
        AND p.module_path = p2.module_path
        AND p.version = p2.version;
END;

ALTER TABLE packages ALTER COLUMN v1_path SET NOT NULL;

-- Drop imports.to_name
ALTER TABLE imports DROP COLUMN to_name;

END;
