-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE symbol_names ADD COLUMN created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE symbol_names ADD COLUMN updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON symbol_names
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

ALTER TABLE package_symbols ADD COLUMN created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE package_symbols ADD COLUMN updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON package_symbols
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

ALTER TABLE symbol_search_documents ADD COLUMN created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE symbol_search_documents ADD COLUMN updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP;
CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON symbol_search_documents
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();

END;
