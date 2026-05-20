-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents DROP COLUMN imported_by_module_count;
ALTER TABLE search_documents DROP COLUMN imported_by_module_count_updated_at;

END;
