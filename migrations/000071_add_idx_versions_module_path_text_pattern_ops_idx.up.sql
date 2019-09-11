-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

CREATE INDEX CONCURRENTLY idx_versions_module_path_text_pattern_ops
	ON versions(module_path text_pattern_ops);
