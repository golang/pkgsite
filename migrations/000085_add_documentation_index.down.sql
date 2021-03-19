-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP INDEX idx_documentation_symbols_documentation_id;
DROP INDEX idx_documentation_goos;
DROP INDEX idx_documentation_goarch;

END;
