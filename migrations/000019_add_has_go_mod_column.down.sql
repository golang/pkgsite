-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions DROP COLUMN has_go_mod;

ALTER TABLE search_documents DROP COLUMN has_go_mod;

END;
