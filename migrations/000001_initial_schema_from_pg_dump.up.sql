-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.
--
-- This schema migration is created from squashing all of our existing
-- migration files in
-- 
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = on;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

CREATE FUNCTION trigger_modify_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;
COMMENT ON FUNCTION trigger_modify_updated_at IS
'FUNCTION trigger_modify_updated_at sets the value of a column named updated_at to the current timestamp. This is used by the versions, packages, and search_documents tables as a trigger to set the value of updated_at.';

CREATE FUNCTION to_tsvector_parent_directories(package_path text, module_path text) RETURNS tsvector
    LANGUAGE plpgsql
    AS $$
  DECLARE
    sub_path TEXT;
    parent_directories TEXT := module_path;
    sub_directories TEXT[][];
    current_directory TEXT := module_path;
    tsv_parent_directories TSVECTOR := module_path::tsvector;
  BEGIN
    IF package_path = module_path THEN
      RETURN tsv_parent_directories;
    END IF;
    -- +2 because substr is one-based and we need to include the trailing slash
    sub_path := substr(package_path, length(module_path) + 2);
    sub_directories := regexp_split_to_array(sub_path, '/');
    FOR i IN 1..cardinality(sub_directories) LOOP
      current_directory := current_directory || '/' || sub_directories[i];
      parent_directories = parent_directories || ' ' || current_directory;
    END LOOP;
    RETURN parent_directories::tsvector;
END;
$$;
COMMENT ON FUNCTION to_tsvector_parent_directories IS
'FUNCTION to_tsvector_parent_directories computes all directories that exist between module_path and package_path, inclusive of both module_path and package_path. Return the result as a tsvector.';

CREATE TYPE version_type AS ENUM (
    'release',
    'prerelease',
    'pseudo'
);
COMMENT ON TYPE version_type IS
'ENUM version_type specifies the version types expected for a given module version.';

CREATE TABLE versions (
    module_path text NOT NULL,
    version text NOT NULL,
    commit_time timestamp with time zone NOT NULL,
    series_path text NOT NULL,
    version_type version_type NOT NULL,
    readme_file_path text,
    readme_contents text,
    source_info jsonb,
    major integer NOT NULL,
    minor integer NOT NULL,
    patch integer NOT NULL,
    prerelease text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (module_path, version)
);
COMMENT ON TABLE versions IS
'TABLE versions contains modules at a specific semantic version.';
COMMENT ON COLUMN versions.prerelease IS
'prerelease will be set as "~" if the version does not have a prerelease tag.';

CREATE INDEX idx_versions_semver_sort ON versions
    USING btree (module_path, major DESC, minor DESC, patch DESC, prerelease DESC);
COMMENT ON INDEX idx_versions_semver_sort IS
'INDEX idx_versions_semver_sort is used to sort versions in order of descending latest. It is used to get the latest version of a package/module and to fetch all versions of a package/module in semver order.';

CREATE INDEX idx_versions_module_path_text_pattern_ops ON versions
    USING btree (module_path text_pattern_ops);
COMMENT ON INDEX idx_versions_module_path_text_pattern_ops IS
'INDEX idx_versions_module_path_text_pattern_ops is using to improve performance of LIKE statements for module_path. It is used to fetch directories matching a given module_path prefix.';

CREATE INDEX idx_versions_version_type ON versions USING btree (version_type);
COMMENT ON INDEX idx_versions_version_type IS
'INDEX idx_versions_version_type is used when fetching versions for a given version_type.';

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON versions
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON versions IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE TABLE packages (
    path text NOT NULL,
    module_path text NOT NULL,
    version text NOT NULL,
    commit_time timestamp with time zone NOT NULL,
    name text NOT NULL,
    synopsis text,
    license_types text[],
    license_paths text[],
    v1_path text NOT NULL,
    goos text NOT NULL,
    goarch text NOT NULL,
    redistributable boolean DEFAULT false NOT NULL,
    documentation text,
    tsv_parent_directories tsvector,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    PRIMARY KEY (path, module_path, version),
    FOREIGN KEY (module_path, version) REFERENCES versions(module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE packages IS
'TABLE packages contains packages in a specific module version.';
COMMENT ON COLUMN packages.commit_time IS
'commit_time is the same as verions.commit_time. It is added here so that we can reduce the number of joins in our queries.';
COMMENT ON COLUMN packages.tsv_parent_directories IS
'tsv_parent_directories should always be NOT NULL, but it is populated by a trigger, so it will be initially NULL on insert.';

CREATE INDEX idx_packages_v1_path ON packages USING btree (v1_path);
COMMENT ON INDEX idx_packages_v1_path IS
'INDEX idx_packages_v1_path is used to get all of the packages in a series.';

CREATE INDEX idx_packages_module_path_text_pattern_ops ON packages USING btree (module_path text_pattern_ops);
COMMENT ON INDEX idx_versions_module_path_text_pattern_ops IS
'INDEX idx_versions_module_path_text_pattern_ops is using to improve performance of LIKE statements for module_path. It is used to fetch directories matching a given module_path prefix.';

CREATE INDEX idx_packages_path_text_pattern_ops ON packages USING btree (path text_pattern_ops);
COMMENT ON INDEX idx_versions_module_path_text_pattern_ops IS
'INDEX idx_versions_module_path_text_pattern_ops is used to improve performance of LIKE statements for module_path. It is used to fetch directories matching a given module_path prefix.';

CREATE INDEX idx_packages_tsv_parent_directories ON packages USING gin (tsv_parent_directories);
COMMENT ON INDEX idx_packages_tsv_parent_directories IS
'INDEX idx_packages_tsv_parent_directories is used to search for packages that match a given prefix. These prefixes are stored as a tsv_vector type in tsv_parent_directories. This is used to fetch all packages in a given directory.';

CREATE FUNCTION trigger_modify_packages_tsv_parent_directories() RETURNS TRIGGER
    LANGUAGE plpgsql
    AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.path, NEW.module_path);
  RETURN NEW;
END;
$$;
COMMENT ON FUNCTION trigger_modify_packages_tsv_parent_directories IS
'FUNCTION trigger_modify_packages_tsv_parent_directories invokes FUNCTION to_tsvector_parent_directories and sets the value of tsv_parent_directories to the output.';

CREATE TRIGGER set_tsv_parent_directories BEFORE INSERT ON packages FOR EACH ROW EXECUTE PROCEDURE trigger_modify_packages_tsv_parent_directories();
COMMENT ON TRIGGER set_tsv_parent_directories ON packages IS
'TRIGGER set_tsv_parent_directories sets the value of tsv_parent_directories to the output of FUNCTION trigger_modify_search_documents_tsv_parent_directories when a new row in inserted.';


CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON packages FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON packages IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE TABLE imports (
    from_path text NOT NULL,
    from_module_path text NOT NULL,
    from_version text NOT NULL,
    to_path text NOT NULL,
    PRIMARY KEY (to_path, from_path, from_version),
    FOREIGN KEY (from_path, from_module_path, from_version)
        REFERENCES packages(path, module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE imports IS
'TABLE imports contains the imports for a package in the packages table. Package (from_path), in module (from_module_path) at version (from_version), imports package (to_path). We do not store the version and module at which to_path is imported because it is hard to compute.';

CREATE TABLE imports_unique (
    to_path text NOT NULL,
    from_path text NOT NULL,
    from_module_path text NOT NULL,
    PRIMARY KEY (to_path, from_path, from_module_path)
);
COMMENT ON TABLE imports_unique IS
'TABLE imports_unique contains the imports for a unique import_path in the packages table. The from_version is dropped; each row says that package from_path in some version of from_module_path imports (some version of) to_path. Used to speed up imported-by computations.';

CREATE TABLE licenses (
    module_path text NOT NULL,
    version text NOT NULL,
    file_path text NOT NULL,
    contents text NOT NULL,
    types text[],
    PRIMARY KEY (module_path, version, file_path),
    FOREIGN KEY (module_path, version) REFERENCES versions(module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE licenses IS
'TABLE licenses contains the license data for a given module version.';

CREATE TABLE excluded_prefixes (
    prefix text NOT NULL,
    created_by text NOT NULL,
    reason text NOT NULL,
    created_at timestamp without time zone DEFAULT NOW(),
    PRIMARY KEY (prefix)
);
COMMENT ON TABLE excluded_prefixes IS
'TABLE excluded_prefixes contains the prefixes of modules or groups of modules we exclude from serving and processing. This is used to deal with attacks.';

CREATE TABLE module_version_states (
    module_path text NOT NULL,
    version text NOT NULL,
    status integer,
    error text,
    try_count integer DEFAULT 0 NOT NULL,
    last_processed_at timestamp with time zone,
    next_processed_after timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    index_timestamp timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    app_version text DEFAULT ''::text NOT NULL,
    PRIMARY KEY (module_path, version)
);
COMMENT ON TABLE module_version_states IS
'TABLE module_version_states is used by the ETL to record the state of every module we have seen from the proxy index.';

CREATE INDEX idx_module_version_states_index_timestamp ON module_version_states USING btree (index_timestamp DESC);
COMMENT ON INDEX idx_module_version_states_index_timestamp IS
'INDEX idx_module_version_states_index_timestamp is used to get the last time a module version was fetched from the the module index.';

CREATE INDEX idx_module_version_states_last_processed_at ON module_version_states USING btree (last_processed_at);
COMMENT ON INDEX idx_module_version_states_last_processed_at IS
'INDEX idx_module_version_states_last_processed_at is used to get the last time a given module version was attempted to be processed.';

CREATE INDEX idx_module_version_states_next_processed_after ON module_version_states USING btree (next_processed_after);
COMMENT ON INDEX idx_module_version_states_last_processed_at IS
'INDEX idx_module_version_states_last_processed_at is used to get the next time at which a module version should be retried for processing.';

CREATE TABLE search_documents (
    package_path text NOT NULL,
    module_path text NOT NULL,
    version text NOT NULL,
    commit_time timestamp with time zone NOT NULL,
    name text NOT NULL,
    synopsis text,
    license_types text[],
    imported_by_count integer DEFAULT 0 NOT NULL,
    redistributable boolean NOT NULL,
    hll_register integer,
    hll_leading_zeros integer,
    tsv_parent_directories tsvector,
    tsv_search_tokens tsvector NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    version_updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    imported_by_count_updated_at timestamp without time zone,
    PRIMARY KEY (package_path),
    FOREIGN KEY (package_path, module_path, version)
        REFERENCES packages(path, module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE search_documents IS
'TABLE search_documents contains a record for the latest version of each package. It is used to generate search results.';
COMMENT ON COLUMN search_documents.hll_register IS
'hll_* columns are added to help implement cardinality estimation using the hyperloglog algorithm. hll_register is the randomized bucket for this record.';
COMMENT ON COLUMN search_documents.hll_leading_zeros IS
'hll_* columns are added to help implement cardinality estimation using the hyperloglog algorithm. hll_leading_zeros is the number of leading zeros in the binary representation of hll_hash(package_path).';

-- INDEX idx_imported_by_count_gt_50 and idx_imported_by_count_gt_8 will be removed
-- and replaced by idx_imported_by_count_desc after testing to compare various search methods.
CREATE INDEX idx_imported_by_count_gt_8 ON search_documents USING btree (package_path) WHERE (imported_by_count > 8);
CREATE INDEX idx_imported_by_count_gt_50 ON search_documents USING btree (package_path) WHERE (imported_by_count > 50);

CREATE INDEX idx_imported_by_count_desc ON search_documents USING btree (imported_by_count DESC);
COMMENT ON INDEX idx_imported_by_count_desc IS
'INDEX idx_imported_by_count_desc is used by popular_search to execute a partial scan of popular search documents.';

CREATE INDEX idx_hll_register_leading_zeros ON search_documents USING btree (hll_register, hll_leading_zeros DESC);
COMMENT ON INDEX idx_hll_register_leading_zeros IS
'INDEX idx_hll_register_leading_zeros allows us to quickly find the maximum number of leading zeros among search documents in each register matching a query, which is necessary for hyperloglog cardinality estimation.';

CREATE INDEX idx_search_documents_imported_by_count_updated_at ON search_documents USING btree (imported_by_count_updated_at);
COMMENT ON INDEX idx_search_documents_imported_by_count_updated_at IS
'INDEX idx_search_documents_imported_by_count_updated_at index is used for incremental update of imported_by counts.';

CREATE INDEX idx_search_documents_module_path_version_package_path ON search_documents
    USING btree (package_path, module_path, version);
COMMENT ON INDEX idx_search_documents_module_path_version_package_path IS
'INDEX idx_search_documents_module_path_version_package_path is used for the FK reference to packages.';

CREATE INDEX idx_search_documents_tsv_parent_directories ON search_documents USING gin (tsv_parent_directories);
COMMENT ON INDEX idx_search_documents_tsv_parent_directories IS
'INDEX idx_search_documents_tsv_parent_directories is used to search for packages that match a given prefix. These prefixes are stored as a tsv_vector type in tsv_parent_directories. This is used to fetch all packages in a given directory.';

CREATE INDEX idx_search_documents_tsv_search_tokens ON search_documents USING gin (tsv_search_tokens);
COMMENT ON INDEX idx_search_documents_tsv_search_tokens IS
'INDEX idx_search_documents_tsv_search_tokens improves performance for full-text search.';

CREATE INDEX idx_search_documents_version_updated_at ON search_documents USING btree (version_updated_at);
COMMENT ON INDEX idx_search_documents_version_updated_at IS
'INDEX idx_search_documents_version_updated_at is used for incremental update of imported_by counts, in order to determine when the latest version of a package was last updated.';

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON search_documents
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON search_documents IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE FUNCTION trigger_modify_search_documents_tsv_parent_directories() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.package_path, NEW.module_path);
  RETURN NEW;
END;
$$;
COMMENT ON FUNCTION trigger_modify_search_documents_tsv_parent_directories IS
'FUNCTION trigger_modify_search_documents_tsv_parent_directories invokes FUNCTION to_tsvector_parent_directories and sets the value of tsv_parent_directories to the output.';

CREATE TRIGGER set_tsv_parent_directories BEFORE INSERT ON search_documents
	FOR EACH ROW EXECUTE PROCEDURE trigger_modify_search_documents_tsv_parent_directories();
COMMENT ON TRIGGER set_tsv_parent_directories ON search_documents IS
'TRIGGER set_tsv_parent_directories sets the value of tsv_parent_directories to the output of FUNCTION trigger_modify_search_documents_tsv_parent_directories when a new row in inserted.';

CREATE FUNCTION hll_hash(text) RETURNS bigint
    LANGUAGE sql
    AS $_$
	-- This is somewhat a hack, since there is no from_hex function in postgres.
	-- Take the first 64 bits of the md5 hash by converting the hexadecimal
	-- string to bitfield, and then bigint.
	SELECT ('x'||substr(md5($1),1,16))::BIT(64)::BIGINT;
$_$;
COMMENT ON FUNCTION hll_hash IS
'FUNCTION hll_hash is a 64-bit integral hash function, which is used in implementing the hyperloglog cardinality estimation algorithm.';

CREATE FUNCTION hll_zeros(bigint) RETURNS integer
    LANGUAGE plpgsql
    AS $_$
BEGIN
	IF $1 < 0 THEN
		RETURN 0;
	END IF;
	-- For bigints, taking log(2, $1) is too inaccurate due to floating point
	-- issues. Specifically log(2, 1<<63-1) == 63.0...
	FOR i IN 0..62 LOOP
		IF ((1::BIGINT<<i) - 1) >= $1 THEN
			RETURN 64-i;
		END IF;
	END LOOP;
	RETURN 1;
END; $_$;
COMMENT ON FUNCTION hll_zeros IS
'FUNCTION hll_zeros returns the number of leading zeros in the binary representation of the given bigint.';

CREATE TYPE search_result AS (
	package_path text,
	module_path text,
	version text,
	commit_time timestamp with time zone,
	imported_by_count integer,
	score double precision
);
COMMENT ON TYPE search_result IS
'TYPE search_result is used to simplify the popular_search function.';

CREATE FUNCTION popular_search(rawquery text, lim integer, off integer) RETURNS SETOF search_result
    LANGUAGE plpgsql
    AS $$
	DECLARE cur CURSOR(query TSQUERY) FOR
		SELECT
			package_path,
			module_path,
			version,
			commit_time,
			imported_by_count,
			(
				ts_rank(tsv_search_tokens, query) *
				ln(exp(1)+imported_by_count) *
				CASE WHEN redistributable THEN 1 ELSE 0.5 END *
				-- Rather than add this `tsv_search_tokens @@ query` check to a
				-- where clause, we simply annihilate the score. Adding it to the
				-- where clause caused the query planner to eventually decide to
				-- use the tsv_search_token gin index rather than the popular
				-- index, which is exactly what this stored proc is trying to
				-- avoid.
				-- It seems like this should be redundant with the ts_rank factor
				-- above, but in fact it is possible for ts_rank to be nonzero, yet
				-- tsv_search_tokens @@ query is false (I think because ts_rank doesn't
				-- have special handling for AND or OR conjunctions).
				CASE WHEN tsv_search_tokens @@ query THEN 1 ELSE 0 END
			) score
			FROM search_documents
			-- This should use the popular document index.
			ORDER BY imported_by_count DESC;
	-- top is the top search results, sorted by score descending, commit time
	-- descending, then package_path ascending.
	top search_result[];
	-- res is the current search result.
	res search_result;
	-- last_idx is the index of the last element in top.
	last_idx INT;
BEGIN
	last_idx := lim+off;
	top := array_fill(NULL::search_result, array[last_idx]);
	OPEN cur(query := websearch_to_tsquery(rawquery));
	FETCH cur INTO res;
	WHILE found LOOP
		IF top[last_idx] IS NULL OR res.score >= top[last_idx].score THEN
			-- Insert res into top, maintaining sort order.
			FOR i IN 1..last_idx LOOP
				-- We want to preserve order by score desc, commit_time desc,
				-- package_path asc, so insert res as soon as it sorted before top[i]
				-- according to this ordering.
				IF top[i] IS NULL OR
					(res.score > top[i].score) OR
					(res.score = top[i].score AND res.commit_time > top[i].commit_time) OR
					(res.score = top[i].score AND res.commit_time = top[i].commit_time AND
					 res.package_path < top[i].package_path) THEN
					top := (top[1:i-1] || res) || top[i:last_idx-1];
					EXIT;
				END IF;
			END LOOP;
		END IF;
		IF top[last_idx].score > ln(exp(1)+res.imported_by_count) THEN
			-- No subsequent document can be scored higher than our lowest scoring
			-- document, as top[last_idx].score > 1.0*ln(e+imported_by_count), and
			-- for all subsequent records ts_rank <= 1.0 and ln(e+imported_by_count)
			-- is monotonically decreasing.
			-- So we're done.
			EXIT;
		END IF;
		FETCH cur INTO res;
	END LOOP;
	CLOSE cur;
	RETURN QUERY SELECT * FROM UNNEST(top[off+1:last_idx])
		WHERE package_path IS NOT NULL AND score > 0.1;
END; $$;
COMMENT ON FUNCTION popular_search IS
'FUNCTION popular_search is used to generate results for search. It is implemented as a stored function, so that we can use a cursor to scan search documents procedurally, and stop scanning early, whenever our search results are provably correct.';

CREATE TEXT SEARCH CONFIGURATION golang (COPY = pg_catalog.english);
ALTER TEXT SEARCH CONFIGURATION golang DROP MAPPING FOR url_path;
ALTER TEXT SEARCH CONFIGURATION golang ALTER MAPPING FOR asciiword WITH simple,english_stem;
COMMENT ON TEXT SEARCH CONFIGURATION golang IS
'TEXT SEARCH CONFIGURATION golang is a custom search configuration used when creating tsvector for search. The url_path token type is remove, so that "github.com/foo/bar@v1.2.3" is indexed only as the full URL string, and not also"/foo/bar@v1.2.3". The asciiword token type is set to a "simple,english_stem" mapping, so that "plural" words will be indexed without stemming. This idea came from the "Morphological and Exact Search" section here: https://asp437.github.io/posts/flexible-fts.html.';
