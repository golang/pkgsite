-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_package_path_id_fkey
    FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_unit_id_fkey
    FOREIGN KEY (unit_id) REFERENCES units(id) ON DELETE CASCADE;

END;
