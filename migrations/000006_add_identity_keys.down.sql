-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER table version_map DROP COLUMN module_id;
ALTER table licenses DROP COLUMN module_id;

END;
