-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE paths ADD COLUMN created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE paths ADD COLUMN updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON paths
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

END;
