-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents DROP COLUMN name;
ALTER TABLE search_documents DROP COLUMN synopsis;
ALTER TABLE search_documents DROP COLUMN license_types;

END;
