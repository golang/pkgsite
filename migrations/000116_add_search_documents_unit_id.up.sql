-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents ADD COLUMN unit_id BIGINT;
CREATE INDEX idx_search_documents_unit_id ON search_documents(unit_id);

END;
