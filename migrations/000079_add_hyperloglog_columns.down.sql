-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_hll_register_leading_zeros;

ALTER TABLE search_documents DROP COLUMN hll_register;
ALTER TABLE search_documents DROP COLUMN hll_leading_zeros;

END;
