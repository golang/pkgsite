-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

SELECT DISTINCT * INTO TEMP temp_duplicates
FROM package_licenses
GROUP BY (module_path, version, file_path, package_path) HAVING count(*) > 1;

DELETE FROM package_licenses pl USING temp_duplicates td
WHERE td.module_path = pl.module_path
  AND td.version = pl.version
  AND td.file_path = pl.file_path
  AND td.package_path = pl.package_path;

ALTER TABLE package_licenses ADD PRIMARY KEY (module_path, version, file_path, package_path);

INSERT INTO package_licenses (SELECT * FROM temp_duplicates);

END;
