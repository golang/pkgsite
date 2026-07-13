-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX IF EXISTS idx_search_documents_embedding;

ALTER TABLE search_documents DROP COLUMN IF EXISTS embedding;

END;
