-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents DROP CONSTRAINT search_documents_pkey;
ALTER TABLE search_documents ADD PRIMARY KEY (package_path_id);

END;
