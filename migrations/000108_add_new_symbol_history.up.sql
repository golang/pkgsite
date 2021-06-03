-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE public.new_symbol_history (
    id bigint NOT NULL PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    package_path_id bigint NOT NULL,
    module_path_id bigint NOT NULL,
    symbol_name_id bigint NOT NULL,
    parent_symbol_name_id bigint NOT NULL,
    package_symbol_id bigint NOT NULL,
    since_version text NOT NULL CHECK ((since_version <> ''::text)),
    sort_version text NOT NULL,
    goos goos NOT NULL,
    goarch goarch NOT NULL,
    UNIQUE (package_path_id, module_path_id, symbol_name_id, goos, goarch),
    FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE
);

CREATE INDEX idx_new_symbol_history_goarch ON new_symbol_history USING btree (goarch);
CREATE INDEX idx_new_symbol_history_goos ON new_symbol_history USING btree (goos);
CREATE INDEX idx_new_symbol_history_module_path_id ON new_symbol_history USING btree (module_path_id);
CREATE INDEX idx_new_symbol_history_parent_symbol_name_id ON new_symbol_history USING btree (parent_symbol_name_id);
CREATE INDEX idx_new_symbol_history_since_version ON new_symbol_history USING btree (since_version);
CREATE INDEX idx_new_symbol_history_sort_version ON new_symbol_history USING btree (sort_version);
CREATE INDEX idx_new_symbol_history_symbol_name_id ON new_symbol_history USING btree (symbol_name_id);

END;
