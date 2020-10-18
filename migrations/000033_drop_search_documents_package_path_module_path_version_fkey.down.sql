-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_package_path_module_path_version_fkey
        FOREIGN KEY (package_path, module_path, version)
        REFERENCES packages(path, module_path, version) ON DELETE CASCADE;

END;
