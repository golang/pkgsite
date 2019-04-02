-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Setting COLLATE "C" avoids non-deterministic sorting based on locale.  This
-- can result in special characters being ignored, which breaks the prerelease
-- sorting semantics.
--
-- See
-- https://www.postgresql.org/message-id/CAL1h%2BQ-6bmpGo2w%2BKvaT7ib4oE4NmoJvvxWp5jH9CaZT%2BM6cKw%40mail.gmail.com
-- for an example of this.
--
-- "C" collation follows ASCII ordering, as described here:
-- https://www.gnu.org/software/libc/manual/html_node/Collation-Functions.html
ALTER TABLE versions ALTER COLUMN prerelease TYPE TEXT COLLATE "C";
ALTER TABLE packages ALTER COLUMN path TYPE TEXT COLLATE "C";
ALTER TABLE packages ALTER COLUMN module_path TYPE TEXT COLLATE "C";

END;
