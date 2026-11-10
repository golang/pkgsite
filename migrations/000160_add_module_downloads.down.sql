-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP MATERIALIZED VIEW IF EXISTS module_downloads_30d;
DROP TABLE IF EXISTS module_download_states;
DROP TABLE IF EXISTS module_downloads;

END;
