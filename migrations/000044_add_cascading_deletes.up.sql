-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE licenses
  DROP CONSTRAINT "licenses_module_path_fkey",
  ADD CONSTRAINT "licenses_module_path_fkey"
    FOREIGN KEY (module_path, version)
    REFERENCES versions(module_path, version)
    ON DELETE CASCADE;

ALTER TABLE packages
  DROP CONSTRAINT "packages_module_path_fkey",
  ADD CONSTRAINT "packages_module_path_fkey"
    FOREIGN KEY (module_path, version)
    REFERENCES versions(module_path, version)
    ON DELETE CASCADE;

ALTER TABLE package_licenses
  DROP CONSTRAINT "package_licenses_module_path_fkey",
  ADD CONSTRAINT "package_licenses_module_path_fkey"
    FOREIGN KEY (module_path, version, package_path)
    REFERENCES packages(module_path, version, path)
    ON DELETE CASCADE,
  DROP CONSTRAINT "package_licenses_module_path_fkey1",
  ADD CONSTRAINT "package_licenses_module_path_fkey1"
    FOREIGN KEY (module_path, version, file_path)
    REFERENCES licenses(module_path, version, file_path)
    ON DELETE CASCADE;

ALTER TABLE documents
  DROP CONSTRAINT "documents_package_path_fkey",
  ADD CONSTRAINT "documents_package_path_fkey"
    FOREIGN KEY (package_path, module_path, version)
    REFERENCES packages(path, module_path, version)
    ON DELETE CASCADE;

ALTER TABLE imports
  DROP CONSTRAINT "imports_from_path_fkey",
  ADD CONSTRAINT "imports_from_path_fkey"
    FOREIGN KEY (from_path, from_module_path, from_version)
    REFERENCES packages(path, module_path, version)
    ON DELETE CASCADE;

END;
