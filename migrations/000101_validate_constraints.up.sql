-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- These commands won't fail and should only set an internal bit in the DB marking the
-- constraint as valid.
-- They don't need to be run in a transaction.
-- See https://www.postgresql.org/docs/12/sql-altertable.html, search for VALIDATE CONSTRAINT.

ALTER TABLE symbol_history VALIDATE CONSTRAINT "symbol_history_module_path_id_fkey";
ALTER TABLE symbol_history VALIDATE CONSTRAINT "symbol_history_package_path_id_fkey";
ALTER TABLE symbol_search_documents VALIDATE CONSTRAINT "symbol_search_documents_package_path_id_fkey";
ALTER TABLE latest_module_versions VALIDATE CONSTRAINT "latest_module_versions_module_path_id_fkey";
