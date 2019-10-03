-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE exclude_prefixes (
       prefix     TEXT PRIMARY KEY,
       created_by TEXT NOT NULL,
       reason     TEXT NOT NULL,
       created_at TIMESTAMP DEFAULT NOW()
);

END;
