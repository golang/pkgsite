-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- As of creation, this index selects around the top 5% most popular packages,
-- for use in optimized search.
CREATE INDEX CONCURRENTLY idx_imported_by_count_gt_8
ON search_documents(package_path)
WHERE imported_by_count > 8;
