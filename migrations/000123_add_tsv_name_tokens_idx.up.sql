-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_symbol_names_tsv_name_tokens ON symbol_names USING gin (tsv_name_tokens);

END;
