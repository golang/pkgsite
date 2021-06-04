-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- These must be run in separate transactions.

ALTER TABLE documentation ADD COLUMN big_unit_id BIGINT;

ALTER TABLE documentation ADD UNIQUE (big_unit_id, goos, goarch);
