-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- See the implementation of estimateResultCount for more explanation of how
-- these are used.

-- The register number for the hyperloglog algorithm, which will be assigned
-- randomly in the range [0, 2^N-1). We precompute this so that we can define
-- the below index, which allows for parallel processing of registers.
ALTER TABLE search_documents ADD COLUMN hll_register integer;
-- The number of leading zeros in binary representation of the row hash.
ALTER TABLE search_documents ADD COLUMN hll_leading_zeros integer;

CREATE INDEX idx_hll_register_leading_zeros ON search_documents(hll_register, hll_leading_zeros DESC);

END;
