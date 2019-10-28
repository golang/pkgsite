-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE excluded_prefixes DROP CONSTRAINT excluded_prefixes_prefix_check;
ALTER TABLE excluded_prefixes DROP CONSTRAINT excluded_prefixes_created_by_check;
ALTER TABLE excluded_prefixes DROP CONSTRAINT excluded_prefixes_reason_check;

END;
