-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_path_documents_tsv_path_tokens ON search_documents USING gin (tsv_path_tokens);

END;
