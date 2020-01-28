-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE versions ADD COLUMN has_go_mod boolean;

COMMENT ON COLUMN versions.has_go_mod IS
'COLUMN has_go_mod records whether the module zip contains a go.mod file.';


ALTER TABLE search_documents ADD COLUMN has_go_mod boolean;

COMMENT ON COLUMN search_documents.has_go_mod IS
'COLUMN has_go_mod records whether the module zip contains a go.mod file.';

END;
