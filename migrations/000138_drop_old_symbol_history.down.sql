-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE old_symbol_history (
    id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    package_path_id integer NOT NULL,
    module_path_id integer NOT NULL,
    symbol_name_id integer NOT NULL,
    parent_symbol_name_id integer NOT NULL,
    package_symbol_id integer NOT NULL,
    since_version text NOT NULL,
    sort_version text NOT NULL,
    goos goos NOT NULL,
    goarch goarch NOT NULL,
    CONSTRAINT symbol_history_since_version_check CHECK ((since_version <> ''::text))
);


ALTER TABLE old_symbol_history OWNER TO postgres;

ALTER TABLE old_symbol_history ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME symbol_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_package_path_id_module_path_id_symbol_name_i_key UNIQUE (package_path_id, module_path_id, symbol_name_id, goos, goarch);


ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_pkey PRIMARY KEY (id);


CREATE INDEX idx_symbol_history_goarch ON old_symbol_history USING btree (goarch);


CREATE INDEX idx_symbol_history_goos ON old_symbol_history USING btree (goos);


CREATE INDEX idx_symbol_history_module_path_id ON old_symbol_history USING btree (module_path_id);


CREATE INDEX idx_symbol_history_parent_symbol_name_id ON old_symbol_history USING btree (parent_symbol_name_id);


CREATE INDEX idx_symbol_history_since_version ON old_symbol_history USING btree (since_version);


CREATE INDEX idx_symbol_history_sort_version ON old_symbol_history USING btree (sort_version);


CREATE INDEX idx_symbol_history_symbol_name_id ON old_symbol_history USING btree (symbol_name_id);


ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_module_path_id_fkey FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE;

ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_package_path_id_fkey FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE;


ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_package_symbol_id_fkey FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE;


ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_parent_symbol_name_id_fkey FOREIGN KEY (parent_symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE;

ALTER TABLE ONLY old_symbol_history
    ADD CONSTRAINT symbol_history_symbol_name_id_fkey FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE;


END;
