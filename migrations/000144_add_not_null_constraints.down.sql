-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE search_documents ALTER COLUMN tsv_path_tokens DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN created_at DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN updated_at DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN updated_at DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN goos DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN goarch DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN goarch DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN package_name DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN package_path DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN uuid_package_name DROP NOT NULL;
ALTER TABLE symbol_search_documents ALTER COLUMN uuid_package_path DROP NOT NULL;

END;
