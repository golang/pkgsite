-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TYPE symbol_type DROP ATTRIBUTE IF EXISTS 'Type';

END;
