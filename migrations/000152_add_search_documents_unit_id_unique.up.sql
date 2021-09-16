-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE UNIQUE INDEX search_documents_unit_id_key ON search_documents(unit_id);

END;
