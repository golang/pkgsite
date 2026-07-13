-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

-- halfvec stores 16-bit half-precision floating-point numbers (FP16).
-- halfvec(256) specifies a 256-dimensional vector (512 bytes per vector).
ALTER TABLE search_documents
ADD COLUMN IF NOT EXISTS embedding halfvec(256);

-- Create HNSW Index for cosine similarity on packages imported by at least one other package.
-- - m (16): Max number of connections/edges per element in each graph layer.
--   Higher values increase search accuracy/recall but increase index size and
--   build time.
-- - ef_construction (64): Size of the dynamic candidate list checked during
--   index construction. Higher values build a better graph for higher search
--   recall at the cost of slower build time.
CREATE INDEX IF NOT EXISTS idx_search_documents_embedding
ON search_documents
USING hnsw (embedding halfvec_cosine_ops)
WITH (m = 16, ef_construction = 64)
-- TODO(golang/go#80242) Update this to use imported_by_module_count.
WHERE imported_by_count >= 1;

END;
