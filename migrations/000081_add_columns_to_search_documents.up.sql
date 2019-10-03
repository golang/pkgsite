-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents ADD COLUMN name TEXT COLLATE "C";
ALTER TABLE search_documents ADD COLUMN synopsis TEXT COLLATE "C";
ALTER TABLE search_documents ADD COLUMN license_types TEXT[];

END;
