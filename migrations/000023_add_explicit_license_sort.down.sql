-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE OR REPLACE VIEW vw_licensed_packages AS
SELECT
  p.*,
  array_agg(l.type) FILTER (WHERE l.version IS NOT NULL) license_types,
  array_agg(l.file_path) FILTER (WHERE l.version IS NOT NULL) license_paths
FROM
  packages p
LEFT JOIN
  package_licenses pl
ON
  p.module_path = pl.module_path
  AND p.version = pl.version
  AND p.path = pl.package_path
LEFT JOIN
  licenses l
ON
  pl.module_path = l.module_path
  AND pl.version = l.version
  AND pl.file_path = l.file_path
GROUP BY (p.module_path, p.version, p.path);
