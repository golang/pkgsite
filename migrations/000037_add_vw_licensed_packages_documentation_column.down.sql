-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP VIEW IF EXISTS vw_licensed_packages CASCADE;
CREATE VIEW vw_licensed_packages AS
 SELECT p.path,
    p.synopsis,
    p.module_path,
    p.version,
    p.name,
    p.major,
    p.minor,
    p.patch,
    p.prerelease,
    p.version_type,
    p.suffix,
    array_agg(l.type ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_types,
    array_agg(l.file_path ORDER BY l.file_path) FILTER (WHERE l.version IS NOT NULL) AS license_paths
   FROM packages p
     LEFT JOIN package_licenses pl ON p.module_path = pl.module_path AND p.version = pl.version AND p.path = pl.package_path
     LEFT JOIN licenses l ON pl.module_path = l.module_path AND pl.version = l.version AND pl.file_path = l.file_path
  GROUP BY p.module_path, p.version, p.path;

END;
