-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_documentation_symbols_documentation_id
    ON documentation_symbols(documentation_id);
CREATE INDEX idx_documentation_goos ON new_documentation(goos);
CREATE INDEX idx_documentation_goarch ON new_documentation(goarch);

END;
