-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_names DROP COLUMN created_at;
ALTER TABLE symbol_names DROP COLUMN updated_at;
ALTER TABLE package_symbols DROP COLUMN created_at;
ALTER TABLE package_symbols DROP COLUMN updated_at;
ALTER TABLE symbol_search_documents DROP COLUMN created_at;
ALTER TABLE symbol_search_documents DROP COLUMN updated_at;

END;
