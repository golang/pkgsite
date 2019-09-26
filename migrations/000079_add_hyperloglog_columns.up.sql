-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- See the implementation of estimateResultCount for more explanation of how
-- these are used.

-- The bucket number for the hyperloglog algorithm, which will be assigned
-- randomly in the range [0, 2^N-1). We precompute this so that we can define
-- the below index, which allows for parallel processing of buckets.
ALTER TABLE search_documents ADD COLUMN hll_bucket integer;
-- The number of leading zeros in binary representation of the row hash.
ALTER TABLE search_documents ADD COLUMN hll_leading_zeros integer;

CREATE INDEX idx_hll_bucket_leading_zeros ON search_documents(hll_bucket, hll_leading_zeros DESC);

END;
