-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- imported_by_module_count is used to track the number of modules that import this package.
ALTER TABLE search_documents
    ADD COLUMN imported_by_module_count
    INTEGER
    DEFAULT 0 NOT NULL;

COMMENT ON COLUMN search_documents.imported_by_module_count
IS 'COLUMN imported_by_module_count is the number of modules that import this package.';

ALTER TABLE search_documents
    ADD COLUMN imported_by_module_count_updated_at
    TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN search_documents.imported_by_module_count_updated_at
IS 'COLUMN imported_by_module_count_updated_at is the time when search_documents.imported_by_module_count is updated.';

CREATE INDEX idx_imported_by_module_count_desc
ON search_documents (imported_by_module_count DESC);

COMMENT ON INDEX idx_imported_by_module_count_desc IS
'INDEX idx_imported_by_module_count_desc is used to scan popular search documents by module count.';

CREATE INDEX idx_search_documents_imported_by_module_count_updated_at
ON search_documents (imported_by_module_count_updated_at);

COMMENT ON INDEX idx_search_documents_imported_by_module_count_updated_at IS
'INDEX idx_search_documents_imported_by_module_count_updated_at is used for incremental updates of module imported_by counts.';

END;
