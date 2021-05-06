-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE INDEX idx_symbol_history_sort_version ON symbol_history(sort_version);
CREATE INDEX idx_symbol_history_goos ON symbol_history(goos);
CREATE INDEX idx_symbol_history_goarch ON symbol_history(goarch);

END;
