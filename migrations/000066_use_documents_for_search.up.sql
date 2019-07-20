-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE search_documents (
  package_path TEXT COLLATE "C" NOT NULL,
  version TEXT COLLATE "C" NOT NULL,
  module_path TEXT COLLATE "C" NOT NULL,
  redistributable BOOLEAN NOT NULL,
  tsv_search_tokens TSVECTOR NOT NULL,
  imported_by_count INTEGER DEFAULT 0 NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
  version_updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
  imported_by_count_updated_at TIMESTAMP,
  PRIMARY KEY (package_path),
  FOREIGN KEY (module_path, version, package_path)
	REFERENCES packages(module_path, version, path)
	ON DELETE CASCADE
);
CREATE INDEX search_documents_tsv_search_tokens_idx
	ON search_documents USING gin(tsv_search_tokens);

CREATE TRIGGER set_updated_at
BEFORE INSERT OR UPDATE ON search_documents
FOR EACH ROW
EXECUTE PROCEDURE trigger_modify_updated_at();

END;
