-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE imports DROP CONSTRAINT imports_pkey;
ALTER TABLE imports ADD PRIMARY KEY (to_path, from_path, from_version);


END;
