-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- This cascading will delete the popular_search function.
DROP TYPE search_result CASCADE;

END;
