-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules DROP CONSTRAINT modules_series_path_fkey;
ALTER TABLE versions DROP CONSTRAINT versions_module_path_fkey;

END;
