-- Copyright 2025 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_mvs_unprocessed_timestamp ON module_version_states (index_timestamp) WHERE last_processed_at IS NULL;

END;
