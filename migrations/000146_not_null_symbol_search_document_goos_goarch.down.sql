-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_search_documents ALTER COLUMN goos DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN goarch DROP NOT NULL;

END;
