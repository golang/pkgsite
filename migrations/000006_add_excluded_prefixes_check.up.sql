-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE excluded_prefixes ADD CHECK (prefix <> '');
ALTER TABLE excluded_prefixes ADD CHECK (reason <> '');
ALTER TABLE excluded_prefixes ADD CHECK (created_by <> '');

END;
