-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER table licenses ADD COLUMN module_id integer REFERENCES modules(id) ON DELETE CASCADE;
ALTER table version_map ADD COLUMN module_id integer;

END;
