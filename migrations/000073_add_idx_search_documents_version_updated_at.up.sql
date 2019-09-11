-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE INDEX CONCURRENTLY idx_search_documents_version_updated_at
	ON search_documents(version_updated_at);
