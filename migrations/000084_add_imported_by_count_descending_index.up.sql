-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE INDEX CONCURRENTLY idx_imported_by_count_desc ON search_documents(imported_by_count DESC);
