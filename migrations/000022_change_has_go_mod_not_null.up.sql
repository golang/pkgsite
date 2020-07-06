-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules ALTER COLUMN has_go_mod SET NOT NULL;
ALTER TABLE search_documents ALTER COLUMN has_go_mod SET NOT NULL;

END;
