-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions
	ADD COLUMN readme TEXT,
	ADD COLUMN synopsis TEXT,
	ADD COLUMN deleted BOOLEAN;

ALTER TABLE packages
	ADD COLUMN major INT,
	ADD COLUMN minor INT,
	ADD COLUMN patch TEXT,
	ADD COLUMN prerelease TEXT,
	ADD COLUMN version_type version_type;

ALTER TABLE documents
	ADD COLUMN deleted BOOLEAN;

CREATE TABLE series (
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    path text NOT NULL
);

CREATE TABLE modules (
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    path text NOT NULL,
    series_path text NOT NULL
);

CREATE TABLE version_logs (
    module_path text NOT NULL,
    version text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    source version_source NOT NULL,
    error text
);

CREATE TABLE package_licenses (
  module_path TEXT NOT NULL,
  version TEXT NOT NULL,
  file_path TEXT NOT NULL,
  package_path TEXT NOT NULL,

  FOREIGN KEY (module_path, version, package_path) REFERENCES packages(module_path, version, path),
  FOREIGN KEY (module_path, version, file_path) REFERENCES licenses(module_path, version, file_path)
);

CREATE OR REPLACE VIEW vw_licensed_packages AS
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
    p.redistributable,
    p.documentation,
    array_agg(l.type ORDER BY l.file_path) FILTER (WHERE (l.version IS NOT NULL)) AS license_types,
    array_agg(l.file_path ORDER BY l.file_path) FILTER (WHERE (l.version IS NOT NULL)) AS license_paths
   FROM ((public.packages p
     LEFT JOIN public.package_licenses pl ON (((p.module_path = pl.module_path) AND (p.version = pl.version) AND (p.path = pl.package_path))))
     LEFT JOIN public.licenses l ON (((pl.module_path = l.module_path) AND (pl.version = l.version) AND (pl.file_path = l.file_path))))
  GROUP BY p.module_path, p.version, p.path;

END;
