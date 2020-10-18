-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents DROP CONSTRAINT IF EXISTS search_documents_package_path_module_path_version_fkey;

END;
