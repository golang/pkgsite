-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE TABLE licenses (
	module_path TEXT NOT NULL,
	version TEXT NOT NULL,
        file_path TEXT NOT NULL, -- path to license file
	contents TEXT NOT NULL,
        type TEXT NOT NULL,

	PRIMARY KEY (module_path, version, file_path),
	FOREIGN KEY (module_path, version) REFERENCES versions(module_path, version)
);

-- package_licenses is join table that associates each package with its
-- applicable licenses.
CREATE TABLE package_licenses (
  module_path TEXT NOT NULL,
  version TEXT NOT NULL,
  file_path TEXT NOT NULL,
  package_path TEXT NOT NULL,

  FOREIGN KEY (module_path, version, package_path) REFERENCES packages(module_path, version, path),
  FOREIGN KEY (module_path, version, file_path) REFERENCES licenses(module_path, version, file_path)
);

-- vw_licensed_packages is a helper view that captures package licenses when
-- querying.
CREATE VIEW vw_licensed_packages AS
SELECT
  p.*,
  -- Aggregate license information into arrays which can later be zipped
  -- together. The FILTER clause here is necessary due to the left-join.
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
