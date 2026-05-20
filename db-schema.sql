-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

--
-- PostgreSQL database dump
--

-- Dumped from database version 14.23 (Debian 14.23-1.pgdg13+1)
-- Dumped by pg_dump version 16.4 (Debian 16.4-3+build4)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'SQL_ASCII';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: postgres
--

-- *not* creating schema, since initdb creates it


ALTER SCHEMA public OWNER TO postgres;

--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner:
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: goarch; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.goarch AS ENUM (
    '386',
    'amd64',
    'arm',
    'arm64',
    'mips',
    'mips64',
    'mips64le',
    'mipsle',
    'ppc64',
    'ppc64le',
    'riscv64',
    's390x',
    'wasm',
    'all'
);


ALTER TYPE public.goarch OWNER TO postgres;

--
-- Name: TYPE goarch; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.goarch IS 'ENUM goarch specifies the execution architecture.';


--
-- Name: goos; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.goos AS ENUM (
    'aix',
    'android',
    'darwin',
    'dragonfly',
    'freebsd',
    'illumos',
    'js',
    'linux',
    'netbsd',
    'openbsd',
    'plan9',
    'solaris',
    'windows',
    'all'
);


ALTER TYPE public.goos OWNER TO postgres;

--
-- Name: TYPE goos; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.goos IS 'ENUM goos specifies the execution operating system.';


--
-- Name: search_result; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.search_result AS (
	package_path text,
	module_path text,
	version text,
	commit_time timestamp with time zone,
	imported_by_count integer,
	score double precision
);


ALTER TYPE public.search_result OWNER TO postgres;

--
-- Name: TYPE search_result; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.search_result IS 'TYPE search_result is used to simplify the popular_search function.';


--
-- Name: symbol_section; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.symbol_section AS ENUM (
    'Constants',
    'Variables',
    'Functions',
    'Types'
);


ALTER TYPE public.symbol_section OWNER TO postgres;

--
-- Name: TYPE symbol_section; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.symbol_section IS 'ENUM symbol_section specifies the section that a symbol appears in on the documentation page.';


--
-- Name: symbol_type; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.symbol_type AS ENUM (
    'Constant',
    'Variable',
    'Function',
    'Struct',
    'Interface',
    'Field',
    'Method',
    'Type'
);


ALTER TYPE public.symbol_type OWNER TO postgres;

--
-- Name: TYPE symbol_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.symbol_type IS 'ENUM symbol_type specifies the type of for a symbol in the symbol_history table.';


--
-- Name: version_type; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.version_type AS ENUM (
    'release',
    'prerelease',
    'pseudo'
);


ALTER TYPE public.version_type OWNER TO postgres;

--
-- Name: TYPE version_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TYPE public.version_type IS 'ENUM version_type specifies the version types expected for a given module version.';


--
-- Name: hll_hash(text); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.hll_hash(text) RETURNS bigint
    LANGUAGE sql PARALLEL SAFE
    AS $_$
	-- This is somewhat a hack, since there is no from_hex function in postgres.
	-- Take the first 64 bits of the md5 hash by converting the hexadecimal
	-- string to bitfield, and then bigint.
	SELECT ('x'||substr(md5($1),1,16))::BIT(64)::BIGINT;
$_$;


ALTER FUNCTION public.hll_hash(text) OWNER TO postgres;

--
-- Name: FUNCTION hll_hash(text); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.hll_hash(text) IS 'FUNCTION hll_hash is a 64-bit integral hash function, which is used in implementing the hyperloglog cardinality estimation algorithm.';


--
-- Name: hll_zeros(bigint); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.hll_zeros(bigint) RETURNS integer
    LANGUAGE plpgsql PARALLEL SAFE
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


ALTER FUNCTION public.hll_zeros(bigint) OWNER TO postgres;

--
-- Name: FUNCTION hll_zeros(bigint); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.hll_zeros(bigint) IS 'FUNCTION hll_zeros returns the number of leading zeros in the binary representation of the given bigint.';


--
-- Name: popular_search(text, integer, integer); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.popular_search(rawquery text, lim integer, off integer) RETURNS SETOF public.search_result
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
				-- package_path asc, so insert res as soon as it is sorted before top[i]
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


ALTER FUNCTION public.popular_search(rawquery text, lim integer, off integer) OWNER TO postgres;

--
-- Name: FUNCTION popular_search(rawquery text, lim integer, off integer); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.popular_search(rawquery text, lim integer, off integer) IS 'FUNCTION popular_search is used to generate results for search. It is implemented as a stored function, so that we can use a cursor to scan search documents procedurally, and stop scanning early, whenever our search results are provably correct.';


--
-- Name: popular_search(text, integer, integer, real, real); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) RETURNS SETOF public.search_result
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
				-- default D, C, B, A weights are {0.1, 0.2, 0.4, 1.0}
				ts_rank('{0.1, 0.2, 1.0, 1.0}', tsv_search_tokens, query) *
				ln(exp(1)+imported_by_count) *
				CASE WHEN redistributable THEN 1 ELSE redist_factor END *
				CASE WHEN COALESCE(has_go_mod, true) THEN 1 ELSE go_mod_factor END *
				CASE WHEN tsv_search_tokens @@ query THEN 1 ELSE 0 END
			) score
			FROM search_documents
			ORDER BY imported_by_count DESC;
	top search_result[];
	res search_result;
	last_idx INT;
BEGIN
	last_idx := lim+off;
	top := array_fill(NULL::search_result, array[last_idx]);
	OPEN cur(query := websearch_to_tsquery(rawquery));
	FETCH cur INTO res;
	WHILE found LOOP
		IF top[last_idx] IS NULL OR res.score >= top[last_idx].score THEN
			FOR i IN 1..last_idx LOOP
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
			EXIT;
		END IF;
		FETCH cur INTO res;
	END LOOP;
	CLOSE cur;
	RETURN QUERY SELECT * FROM UNNEST(top[off+1:last_idx])
		WHERE package_path IS NOT NULL AND score > 0.1;
END; $$;


ALTER FUNCTION public.popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) OWNER TO postgres;

--
-- Name: FUNCTION popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) IS 'FUNCTION popular_search is used to generate results for search. It is implemented as a stored function, so that we can use a cursor to scan search documents procedurally, and stop scanning early, whenever our search results are provably correct.';


--
-- Name: popular_search_go_mod(text, integer, integer, real, real); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.popular_search_go_mod(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) RETURNS SETOF public.search_result
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
				CASE WHEN redistributable THEN 1 ELSE redist_factor END *
				CASE WHEN COALESCE(has_go_mod, true) THEN 1 ELSE go_mod_factor END *
				CASE WHEN tsv_search_tokens @@ query THEN 1 ELSE 0 END
			) score
			FROM search_documents
			ORDER BY imported_by_count DESC;
	top search_result[];
	res search_result;
	last_idx INT;
BEGIN
	last_idx := lim+off;
	top := array_fill(NULL::search_result, array[last_idx]);
	OPEN cur(query := websearch_to_tsquery(rawquery));
	FETCH cur INTO res;
	WHILE found LOOP
		IF top[last_idx] IS NULL OR res.score >= top[last_idx].score THEN
			FOR i IN 1..last_idx LOOP
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
			EXIT;
		END IF;
		FETCH cur INTO res;
	END LOOP;
	CLOSE cur;
	RETURN QUERY SELECT * FROM UNNEST(top[off+1:last_idx])
		WHERE package_path IS NOT NULL AND score > 0.1;
END; $$;


ALTER FUNCTION public.popular_search_go_mod(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) OWNER TO postgres;

--
-- Name: FUNCTION popular_search_go_mod(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.popular_search_go_mod(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) IS 'FUNCTION popular_search_go_mod is identical to popular_search except for the additional multiplier for the has_go_mod field.';


--
-- Name: set_big_id(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.set_big_id() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Update big_id with the same value used for id.
    NEW.big_id = NEW.id;
    RETURN NEW;
END
$$;


ALTER FUNCTION public.set_big_id() OWNER TO postgres;

--
-- Name: set_big_path_ids(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.set_big_path_ids() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.big_package_path_id = NEW.package_path_id;
    NEW.big_module_path_id = NEW.module_path_id;
    RETURN NEW;
END
$$;


ALTER FUNCTION public.set_big_path_ids() OWNER TO postgres;

--
-- Name: set_tsv_name_tokens(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.set_tsv_name_tokens() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.tsv_name_tokens =
        -- Index full identifier name.
        SETWEIGHT(TO_TSVECTOR('symbols', replace(NEW.name, '_', '-')), 'C') ||
        -- Index <identifier> without parent name (i.e. "Begin" in
        -- "DB.Begin").
        -- This is weighted less, so that if other symbols are just named
        -- "Begin" they will rank higher in a search for "Begin".
        SETWEIGHT(
            TO_TSVECTOR('symbols', split_part(replace(NEW.name, '_', '-'), '.', 2)),
            'D');
    RETURN NEW;
END
$$;


ALTER FUNCTION public.set_tsv_name_tokens() OWNER TO postgres;

--
-- Name: trigger_modify_ln_imported_by_count(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_ln_imported_by_count() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.ln_imported_by_count = ln(exp(1)+NEW.imported_by_count);
    RETURN NEW;
END
$$;


ALTER FUNCTION public.trigger_modify_ln_imported_by_count() OWNER TO postgres;

--
-- Name: trigger_modify_search_documents_tsv_parent_directories(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_search_documents_tsv_parent_directories() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
  BEGIN
    NEW.tsv_parent_directories = to_tsvector_parent_directories(NEW.package_path, NEW.module_path);
  RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_modify_search_documents_tsv_parent_directories() OWNER TO postgres;

--
-- Name: FUNCTION trigger_modify_search_documents_tsv_parent_directories(); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.trigger_modify_search_documents_tsv_parent_directories() IS 'FUNCTION trigger_modify_search_documents_tsv_parent_directories invokes FUNCTION to_tsvector_parent_directories and sets the value of tsv_parent_directories to the output.';


--
-- Name: trigger_modify_symbol_search_documents_imported_by_count(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_symbol_search_documents_imported_by_count() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE symbol_search_documents ssd
    SET imported_by_count=NEW.imported_by_count
    WHERE ssd.unit_id=NEW.unit_id;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_modify_symbol_search_documents_imported_by_count() OWNER TO postgres;

--
-- Name: trigger_modify_updated_at(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;


ALTER FUNCTION public.trigger_modify_updated_at() OWNER TO postgres;

--
-- Name: FUNCTION trigger_modify_updated_at(); Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON FUNCTION public.trigger_modify_updated_at() IS 'FUNCTION trigger_modify_updated_at sets the value of a column named updated_at to the current timestamp. This is used by the versions, packages, and search_documents tables as a trigger to set the value of updated_at.';


--
-- Name: trigger_modify_uuid_package_name(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_uuid_package_name() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.uuid_package_name = uuid_generate_v5(uuid_nil(), NEW.package_name);
    RETURN NEW;
END
$$;


ALTER FUNCTION public.trigger_modify_uuid_package_name() OWNER TO postgres;

--
-- Name: trigger_modify_uuid_package_path(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.trigger_modify_uuid_package_path() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.uuid_package_path = uuid_generate_v5(uuid_nil(), NEW.package_path);
    RETURN NEW;
END
$$;


ALTER FUNCTION public.trigger_modify_uuid_package_path() OWNER TO postgres;

--
-- Name: update_documentation_id(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.update_documentation_id() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.id=nextval('sequence_documentation_id');
    -- Update id_bigint with the same value on insert/update.
    NEW.id_bigint=NEW.id;
    RETURN NEW;
END
$$;


ALTER FUNCTION public.update_documentation_id() OWNER TO postgres;

--
-- Name: path_tokens; Type: TEXT SEARCH CONFIGURATION; Schema: public; Owner: postgres
--

CREATE TEXT SEARCH CONFIGURATION public.path_tokens (
    PARSER = pg_catalog."default" );

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR asciiword WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR word WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR numword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR email WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR url WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR host WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR sfloat WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR version WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR hword_numpart WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR hword_part WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR numhword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR asciihword WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR hword WITH english_stem;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR url_path WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR file WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR "float" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR "int" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.path_tokens
    ADD MAPPING FOR uint WITH simple;


ALTER TEXT SEARCH CONFIGURATION public.path_tokens OWNER TO postgres;

--
-- Name: TEXT SEARCH CONFIGURATION path_tokens; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TEXT SEARCH CONFIGURATION public.path_tokens IS 'TEXT SEARCH CONFIGURATION path_tokens is a custom search configuration used when creating a tsvector
from tokens that we generate from a path. The configuration ignores items that are part of a hyphenated
word, because our token generator already splits at hyphens.';


--
-- Name: symbols; Type: TEXT SEARCH CONFIGURATION; Schema: public; Owner: postgres
--

CREATE TEXT SEARCH CONFIGURATION public.symbols (
    PARSER = pg_catalog."default" );

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR asciiword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR word WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR numword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR email WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR url WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR host WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR sfloat WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR version WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR numhword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR asciihword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR hword WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR file WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR "float" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR "int" WITH simple;

ALTER TEXT SEARCH CONFIGURATION public.symbols
    ADD MAPPING FOR uint WITH simple;


ALTER TEXT SEARCH CONFIGURATION public.symbols OWNER TO postgres;

--
-- Name: TEXT SEARCH CONFIGURATION symbols; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TEXT SEARCH CONFIGURATION public.symbols IS 'TEXT SEARCH CONFIGURATION symbols is a custom search configuration used for symbol search. The configuration ignores items that are part of a hyphenated word and url_parts. These are handled in the code.';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: documentation; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.documentation (
    id bigint NOT NULL,
    goos public.goos NOT NULL,
    goarch public.goarch NOT NULL,
    synopsis text NOT NULL,
    source bytea,
    unit_id bigint NOT NULL
);


ALTER TABLE public.documentation OWNER TO postgres;

--
-- Name: TABLE documentation; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.documentation IS 'TABLE documentation contains documentation for packages in the database.';


--
-- Name: COLUMN documentation.source; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.documentation.source IS 'COLUMN source contains the encoded ast.Files for the package.';


--
-- Name: documentation_symbols; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.documentation_symbols (
    id bigint NOT NULL,
    documentation_id bigint NOT NULL,
    package_symbol_id bigint NOT NULL
);


ALTER TABLE public.documentation_symbols OWNER TO postgres;

--
-- Name: TABLE documentation_symbols; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.documentation_symbols IS 'TABLE documentation_symbols contains symbols for a given row in the documentation table.';


--
-- Name: legacy_documentation_symbols; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.legacy_documentation_symbols (
    documentation_id integer NOT NULL,
    package_symbol_id integer NOT NULL,
    id bigint NOT NULL
);


ALTER TABLE public.legacy_documentation_symbols OWNER TO postgres;

--
-- Name: TABLE legacy_documentation_symbols; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.legacy_documentation_symbols IS 'TABLE documentation_symbols contains symbols for a given row in the documentation table.';


--
-- Name: documentation_symbols_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.legacy_documentation_symbols ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.documentation_symbols_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: documentation_symbols_id_seq1; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.documentation_symbols ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.documentation_symbols_id_seq1
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: excluded_prefixes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.excluded_prefixes (
    prefix text NOT NULL,
    created_by text NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT excluded_prefixes_created_by_check CHECK ((created_by <> ''::text)),
    CONSTRAINT excluded_prefixes_prefix_check CHECK ((prefix <> ''::text)),
    CONSTRAINT excluded_prefixes_reason_check CHECK ((reason <> ''::text))
);


ALTER TABLE public.excluded_prefixes OWNER TO postgres;

--
-- Name: TABLE excluded_prefixes; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.excluded_prefixes IS 'TABLE excluded_prefixes contains the prefixes of modules or groups of modules we exclude from serving and processing. This is used to deal with attacks.';


--
-- Name: imports; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.imports (
    unit_id bigint NOT NULL,
    to_path_id bigint NOT NULL
);


ALTER TABLE public.imports OWNER TO postgres;

--
-- Name: TABLE imports; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.imports IS 'TABLE imports contains the imports for a package in the units table.
The package represented by unit_id imports to_path_id.
We do not store the version and module at which to_path is imported because it is hard to compute.';


--
-- Name: imports_unique; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.imports_unique (
    to_path text NOT NULL,
    from_path text NOT NULL,
    from_module_path text NOT NULL
);


ALTER TABLE public.imports_unique OWNER TO postgres;

--
-- Name: TABLE imports_unique; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.imports_unique IS 'TABLE imports_unique contains the imports for a unique import_path in the packages table. The from_version is dropped; each row says that package from_path in some version of from_module_path imports (some version of) to_path. Used to speed up imported-by computations.';


--
-- Name: latest_module_versions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.latest_module_versions (
    module_path_id bigint NOT NULL,
    raw_version text NOT NULL,
    cooked_version text NOT NULL,
    good_version text NOT NULL,
    raw_go_mod_bytes bytea NOT NULL,
    status integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    series_path text,
    deprecated boolean
);


ALTER TABLE public.latest_module_versions OWNER TO postgres;

--
-- Name: TABLE latest_module_versions; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.latest_module_versions IS 'TABLE latest_module_versions holds the latest versions of a module.';


--
-- Name: COLUMN latest_module_versions.raw_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.raw_version IS 'COLUMN raw_version is the latest version of the module, ignoring retractions.';


--
-- Name: COLUMN latest_module_versions.cooked_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.cooked_version IS 'COLUMN cooked_version is the latest unretracted version of the module.';


--
-- Name: COLUMN latest_module_versions.good_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.good_version IS 'COLUMN good_version is the latest version of the module with a 2xx status.';


--
-- Name: COLUMN latest_module_versions.raw_go_mod_bytes; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.raw_go_mod_bytes IS 'COLUMN raw_go_mod_bytes is the contents of the go.mod file for the given module and raw version.';


--
-- Name: COLUMN latest_module_versions.status; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.status IS 'COLUMN status holds the status of the operations used to determine latest versions.';


--
-- Name: COLUMN latest_module_versions.updated_at; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.latest_module_versions.updated_at IS 'COLUMN updated_at tracks the time that the row was last changed.';


--
-- Name: licenses; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.licenses (
    file_path text NOT NULL,
    contents text NOT NULL,
    types text[],
    coverage jsonb,
    module_id integer NOT NULL
);


ALTER TABLE public.licenses OWNER TO postgres;

--
-- Name: TABLE licenses; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.licenses IS 'TABLE licenses contains the license data for a given module version.';


--
-- Name: COLUMN licenses.coverage; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.licenses.coverage IS 'COLUMN coverage contains the JSON-serialized contents of the licensecheck.Coverage value returned from calling licensecheck.Cover.';


--
-- Name: module_version_states; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.module_version_states (
    module_path text NOT NULL,
    version text NOT NULL,
    status integer DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    try_count integer DEFAULT 0 NOT NULL,
    last_processed_at timestamp with time zone,
    next_processed_after timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    index_timestamp timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    app_version text DEFAULT ''::text NOT NULL,
    sort_version text NOT NULL,
    go_mod_path text DEFAULT ''::text NOT NULL,
    num_packages integer,
    incompatible boolean NOT NULL,
    has_go_mod boolean
);


ALTER TABLE public.module_version_states OWNER TO postgres;

--
-- Name: TABLE module_version_states; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.module_version_states IS 'TABLE module_version_states is used by the ETL to record the state of every module we have seen from the proxy index.';


--
-- Name: COLUMN module_version_states.sort_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.module_version_states.sort_version IS 'COLUMN sort_version holds the version in a form suitable for use in ORDER BY. The string format is described in internal/version.ForSorting.';


--
-- Name: COLUMN module_version_states.go_mod_path; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.module_version_states.go_mod_path IS 'COLUMN go_mod_path holds the module path from the go.mod file.';


--
-- Name: COLUMN module_version_states.incompatible; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.module_version_states.incompatible IS 'COLUMN incompatible defines whether the the version for the given module is incompatible';


--
-- Name: modules; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.modules (
    module_path text NOT NULL,
    version text NOT NULL,
    commit_time timestamp with time zone NOT NULL,
    series_path text NOT NULL,
    version_type public.version_type NOT NULL,
    source_info jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    sort_version text NOT NULL,
    redistributable boolean NOT NULL,
    has_go_mod boolean NOT NULL,
    id integer NOT NULL,
    incompatible boolean NOT NULL,
    status integer
);


ALTER TABLE public.modules OWNER TO postgres;

--
-- Name: TABLE modules; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.modules IS 'TABLE modules contains modules at a specific semantic version.';


--
-- Name: COLUMN modules.sort_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.modules.sort_version IS 'COLUMN sort_version holds the version in a form suitable for use in ORDER BY.';


--
-- Name: COLUMN modules.redistributable; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.modules.redistributable IS 'COLUMN redistributable says whether the module is redistributable.';


--
-- Name: COLUMN modules.has_go_mod; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.modules.has_go_mod IS 'COLUMN has_go_mod records whether the module zip contains a go.mod file.';


--
-- Name: COLUMN modules.incompatible; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.modules.incompatible IS 'COLUMN incompatible defines whether the the version for the given module is incompatible';


--
-- Name: COLUMN modules.status; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.modules.status IS 'COLUMN status describes the status of the module in the database. This status will match module_version_states.status.';


--
-- Name: modules_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.modules ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.modules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: new_documentation_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.documentation ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.new_documentation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: symbol_history; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.symbol_history (
    id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    package_path_id bigint NOT NULL,
    module_path_id bigint NOT NULL,
    symbol_name_id bigint NOT NULL,
    parent_symbol_name_id bigint NOT NULL,
    package_symbol_id bigint NOT NULL,
    since_version text NOT NULL,
    sort_version text NOT NULL,
    goos public.goos NOT NULL,
    goarch public.goarch NOT NULL,
    CONSTRAINT new_symbol_history_since_version_check CHECK ((since_version <> ''::text))
);


ALTER TABLE public.symbol_history OWNER TO postgres;

--
-- Name: new_symbol_history_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.symbol_history ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.new_symbol_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: package_symbols; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.package_symbols (
    package_path_id bigint NOT NULL,
    module_path_id bigint NOT NULL,
    symbol_name_id integer NOT NULL,
    parent_symbol_name_id integer NOT NULL,
    section public.symbol_section NOT NULL,
    type public.symbol_type NOT NULL,
    synopsis text NOT NULL,
    id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.package_symbols OWNER TO postgres;

--
-- Name: TABLE package_symbols; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.package_symbols IS 'TABLE package_symbols contains information that fully describes symbols that appear in a given package.';


--
-- Name: COLUMN package_symbols.parent_symbol_name_id; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.package_symbols.parent_symbol_name_id IS 'COLUMN package_symbols.parent_symbol_name_id indicates the parent type for a symbol. If the symbol is the parent type, the parent_symbol_id will be equal to the symbol_id.';


--
-- Name: package_symbols_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.package_symbols ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.package_symbols_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: package_version_states; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.package_version_states (
    package_path text NOT NULL,
    module_path text NOT NULL,
    version text NOT NULL,
    status integer NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.package_version_states OWNER TO postgres;

--
-- Name: TABLE package_version_states; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.package_version_states IS 'TABLE package_version_states is used to record the state of every package we have seen from the proxy.';


--
-- Name: paths; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.paths (
    path text NOT NULL,
    id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.paths OWNER TO postgres;

--
-- Name: TABLE paths; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.paths IS 'TABLE paths contains the path string for every path in the units table.';


--
-- Name: units; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.units (
    id bigint NOT NULL,
    module_id integer NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    license_types text[],
    license_paths text[],
    redistributable boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    path_id integer NOT NULL,
    v1path_id integer NOT NULL
);


ALTER TABLE public.units OWNER TO postgres;

--
-- Name: TABLE units; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.units IS 'TABLE units contains every module, package and directory path at every version.';


--
-- Name: paths_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.units ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.paths_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: paths_id_seq1; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.paths ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.paths_id_seq1
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: readmes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.readmes (
    unit_id bigint NOT NULL,
    file_path text NOT NULL,
    contents text NOT NULL
);


ALTER TABLE public.readmes OWNER TO postgres;

--
-- Name: TABLE readmes; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.readmes IS 'TABLE readmes contains README files at a given path.';


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


ALTER TABLE public.schema_migrations OWNER TO postgres;

--
-- Name: search_documents; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.search_documents (
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
    tsv_search_tokens tsvector NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    version_updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    imported_by_count_updated_at timestamp with time zone,
    has_go_mod boolean NOT NULL,
    module_path_id integer,
    package_path_id bigint NOT NULL,
    unit_id bigint NOT NULL,
    path_tokens text,
    tsv_path_tokens tsvector NOT NULL,
    ln_imported_by_count numeric NOT NULL
);


ALTER TABLE public.search_documents OWNER TO postgres;

--
-- Name: TABLE search_documents; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.search_documents IS 'TABLE search_documents contains a record for the latest version of each package. It is used to generate search results.';


--
-- Name: COLUMN search_documents.hll_register; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.search_documents.hll_register IS 'hll_* columns are added to help implement cardinality estimation using the hyperloglog algorithm. hll_register is the randomized bucket for this record.';


--
-- Name: COLUMN search_documents.hll_leading_zeros; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.search_documents.hll_leading_zeros IS 'hll_* columns are added to help implement cardinality estimation using the hyperloglog algorithm. hll_leading_zeros is the number of leading zeros in the binary representation of hll_hash(package_path).';


--
-- Name: COLUMN search_documents.has_go_mod; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.search_documents.has_go_mod IS 'COLUMN has_go_mod records whether the module zip contains a go.mod file.';


--
-- Name: symbol_names; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.symbol_names (
    id integer NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.symbol_names OWNER TO postgres;

--
-- Name: TABLE symbol_names; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.symbol_names IS 'TABLE symbols contains all of the symbol names in the database. The name for a field or method expression is the <type-name>.<field-or-method-name>.';


--
-- Name: symbol_search_documents; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.symbol_search_documents (
    id bigint NOT NULL,
    package_path_id bigint NOT NULL,
    symbol_name_id bigint NOT NULL,
    unit_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    package_symbol_id bigint NOT NULL,
    goos public.goos NOT NULL,
    goarch public.goarch NOT NULL,
    package_name text NOT NULL,
    uuid_package_name uuid NOT NULL,
    package_path text NOT NULL,
    uuid_package_path uuid NOT NULL,
    imported_by_count integer DEFAULT 0 NOT NULL,
    ln_imported_by_count numeric,
    symbol_name text NOT NULL
);


ALTER TABLE public.symbol_search_documents OWNER TO postgres;

--
-- Name: TABLE symbol_search_documents; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.symbol_search_documents IS 'TABLE symbol_search_documents contains data used to search for symbols. A row exists for the latest version of each package_path and each exported symbol in that package. Each symbol maps to a package in search_documents.';


--
-- Name: symbol_search_documents_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.symbol_search_documents ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.symbol_search_documents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: symbols_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

ALTER TABLE public.symbol_names ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.symbols_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: version_map; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.version_map (
    module_path text NOT NULL,
    requested_version text NOT NULL,
    resolved_version text,
    status integer NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    sort_version text,
    module_id integer,
    go_mod_path text NOT NULL
);


ALTER TABLE public.version_map OWNER TO postgres;

--
-- Name: TABLE version_map; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.version_map IS 'TABLE version_map contains data about a user-requested path and the semantic version that it resolves to. It is used to support fetching frontend detail pages using module queries.';


--
-- Name: COLUMN version_map.requested_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.version_map.requested_version IS 'COLUMN requested_version is the version that was requested by a user from the frontend. It may or may not resolve to a semantic version.';


--
-- Name: COLUMN version_map.resolved_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.version_map.resolved_version IS 'COLUMN resolved_version is the semantic version that a requested_version resolves to.';


--
-- Name: COLUMN version_map.status; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.version_map.status IS 'COLUMN status is the status returned by the ETL when fetching the module version.';


--
-- Name: COLUMN version_map.error; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.version_map.error IS 'COLUMN status is the error that occurred when fetching the module version, in cases when status != 200.';


--
-- Name: documentation documentation_big_unit_id_goos_goarch_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation
    ADD CONSTRAINT documentation_big_unit_id_goos_goarch_key UNIQUE (unit_id, goos, goarch);


--
-- Name: documentation documentation_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation
    ADD CONSTRAINT documentation_pkey PRIMARY KEY (id);


--
-- Name: legacy_documentation_symbols documentation_symbols_documentation_id_package_symbol_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.legacy_documentation_symbols
    ADD CONSTRAINT documentation_symbols_documentation_id_package_symbol_id_key UNIQUE (documentation_id, package_symbol_id);


--
-- Name: documentation_symbols documentation_symbols_documentation_id_package_symbol_id_key1; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation_symbols
    ADD CONSTRAINT documentation_symbols_documentation_id_package_symbol_id_key1 UNIQUE (documentation_id, package_symbol_id);


--
-- Name: legacy_documentation_symbols documentation_symbols_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.legacy_documentation_symbols
    ADD CONSTRAINT documentation_symbols_pkey PRIMARY KEY (id);


--
-- Name: documentation_symbols documentation_symbols_pkey1; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation_symbols
    ADD CONSTRAINT documentation_symbols_pkey1 PRIMARY KEY (id);


--
-- Name: excluded_prefixes excluded_prefixes_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.excluded_prefixes
    ADD CONSTRAINT excluded_prefixes_pkey PRIMARY KEY (prefix);


--
-- Name: imports imports_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.imports
    ADD CONSTRAINT imports_pkey PRIMARY KEY (unit_id, to_path_id);


--
-- Name: imports_unique imports_unique_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.imports_unique
    ADD CONSTRAINT imports_unique_pkey PRIMARY KEY (to_path, from_path, from_module_path);


--
-- Name: latest_module_versions latest_module_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.latest_module_versions
    ADD CONSTRAINT latest_module_versions_pkey PRIMARY KEY (module_path_id);


--
-- Name: licenses licenses_module_id_file_path; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.licenses
    ADD CONSTRAINT licenses_module_id_file_path UNIQUE (module_id, file_path);


--
-- Name: licenses licenses_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.licenses
    ADD CONSTRAINT licenses_pkey PRIMARY KEY (module_id, file_path);


--
-- Name: module_version_states module_version_states_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.module_version_states
    ADD CONSTRAINT module_version_states_pkey PRIMARY KEY (module_path, version);


--
-- Name: modules modules_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_id_key UNIQUE (id);


--
-- Name: modules modules_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_pkey PRIMARY KEY (module_path, version);


--
-- Name: symbol_history new_symbol_history_package_path_id_module_path_id_symbol_na_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_package_path_id_module_path_id_symbol_na_key UNIQUE (package_path_id, module_path_id, symbol_name_id, goos, goarch);


--
-- Name: symbol_history new_symbol_history_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_pkey PRIMARY KEY (id);


--
-- Name: package_symbols package_symbols_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.package_symbols
    ADD CONSTRAINT package_symbols_pkey PRIMARY KEY (id);


--
-- Name: package_version_states package_version_states_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.package_version_states
    ADD CONSTRAINT package_version_states_pkey PRIMARY KEY (package_path, module_path, version);


--
-- Name: paths paths_big_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.paths
    ADD CONSTRAINT paths_big_id_key UNIQUE (id);


--
-- Name: paths paths_path_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.paths
    ADD CONSTRAINT paths_path_key UNIQUE (path);


--
-- Name: paths paths_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.paths
    ADD CONSTRAINT paths_pkey PRIMARY KEY (id);


--
-- Name: readmes readmes_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.readmes
    ADD CONSTRAINT readmes_pkey PRIMARY KEY (unit_id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: search_documents search_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.search_documents
    ADD CONSTRAINT search_documents_pkey PRIMARY KEY (package_path_id);


--
-- Name: symbol_search_documents symbol_search_documents_package_path_id_symbol_name_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_package_path_id_symbol_name_id_key UNIQUE (package_path_id, symbol_name_id);


--
-- Name: symbol_search_documents symbol_search_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_pkey PRIMARY KEY (id);


--
-- Name: symbol_names symbols_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_names
    ADD CONSTRAINT symbols_name_key UNIQUE (name);


--
-- Name: symbol_names symbols_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_names
    ADD CONSTRAINT symbols_pkey PRIMARY KEY (id);


--
-- Name: units units_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units
    ADD CONSTRAINT units_pkey PRIMARY KEY (id);


--
-- Name: version_map version_map_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.version_map
    ADD CONSTRAINT version_map_pkey PRIMARY KEY (module_path, requested_version);


--
-- Name: idx_documentation_goarch; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_documentation_goarch ON public.documentation USING btree (goarch);


--
-- Name: idx_documentation_goos; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_documentation_goos ON public.documentation USING btree (goos);


--
-- Name: idx_documentation_symbols_documentation_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_documentation_symbols_documentation_id ON public.documentation_symbols USING btree (documentation_id);


--
-- Name: idx_documentation_symbols_package_symbol_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_documentation_symbols_package_symbol_id ON public.documentation_symbols USING btree (package_symbol_id);


--
-- Name: idx_hll_register_leading_zeros; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_hll_register_leading_zeros ON public.search_documents USING btree (hll_register, hll_leading_zeros DESC);


--
-- Name: INDEX idx_hll_register_leading_zeros; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_hll_register_leading_zeros IS 'INDEX idx_hll_register_leading_zeros allows us to quickly find the maximum number of leading zeros among search documents in each register matching a query, which is necessary for hyperloglog cardinality estimation.';


--
-- Name: idx_imported_by_count_desc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_imported_by_count_desc ON public.search_documents USING btree (imported_by_count DESC);


--
-- Name: INDEX idx_imported_by_count_desc; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_imported_by_count_desc IS 'INDEX idx_imported_by_count_desc is used by popular_search to execute a partial scan of popular search documents.';


--
-- Name: idx_imports_to_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_imports_to_path_id ON public.imports USING btree (to_path_id);


--
-- Name: idx_imports_unique_from_module_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_imports_unique_from_module_path ON public.imports_unique USING btree (from_module_path);


--
-- Name: idx_latest_module_versions_series_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_latest_module_versions_series_path ON public.latest_module_versions USING btree (series_path);


--
-- Name: idx_latest_module_versions_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_latest_module_versions_status ON public.latest_module_versions USING btree (status);


--
-- Name: idx_legacy_documentation_symbols_documentation_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_legacy_documentation_symbols_documentation_id ON public.legacy_documentation_symbols USING btree (documentation_id);


--
-- Name: idx_legacy_documentation_symbols_package_symbol_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_legacy_documentation_symbols_package_symbol_id ON public.legacy_documentation_symbols USING btree (package_symbol_id);


--
-- Name: idx_module_version_states_incompatible; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_incompatible ON public.module_version_states USING btree (incompatible);


--
-- Name: INDEX idx_module_version_states_incompatible; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_module_version_states_incompatible IS 'INDEX idx_module_version_states_incompatible is used to sort versions if they are incompatible';


--
-- Name: idx_module_version_states_index_timestamp; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_index_timestamp ON public.module_version_states USING btree (index_timestamp DESC);


--
-- Name: INDEX idx_module_version_states_index_timestamp; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_module_version_states_index_timestamp IS 'INDEX idx_module_version_states_index_timestamp is used to get the last time a module version was fetched from the the module index.';


--
-- Name: idx_module_version_states_last_processed_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_last_processed_at ON public.module_version_states USING btree (last_processed_at);


--
-- Name: INDEX idx_module_version_states_last_processed_at; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_module_version_states_last_processed_at IS 'INDEX idx_module_version_states_last_processed_at is used to get the next time at which a module version should be retried for processing.';


--
-- Name: idx_module_version_states_next_processed_after; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_next_processed_after ON public.module_version_states USING btree (next_processed_after);


--
-- Name: INDEX idx_module_version_states_next_processed_after; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_module_version_states_next_processed_after IS 'INDEX idx_module_version_states_next_processed_after is used to get the next time at which a module version should be retried for processing.';


--
-- Name: idx_module_version_states_num_packages; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_num_packages ON public.module_version_states USING btree (num_packages);


--
-- Name: idx_module_version_states_sort_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_sort_version ON public.module_version_states USING btree (sort_version DESC);


--
-- Name: INDEX idx_module_version_states_sort_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_module_version_states_sort_version IS 'INDEX idx_module_version_states_sort_version is used to sort by version, to determine when a module version should be retried for processing.';


--
-- Name: idx_module_version_states_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_module_version_states_status ON public.module_version_states USING btree (status);


--
-- Name: idx_modules_incompatible; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_modules_incompatible ON public.modules USING btree (incompatible);


--
-- Name: INDEX idx_modules_incompatible; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_modules_incompatible IS 'INDEX idx_modules_incompatible is used to sort versions if they are incompatible';


--
-- Name: idx_modules_module_path_text_pattern_ops; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_modules_module_path_text_pattern_ops ON public.modules USING btree (module_path text_pattern_ops);


--
-- Name: INDEX idx_modules_module_path_text_pattern_ops; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_modules_module_path_text_pattern_ops IS 'INDEX idx_versions_module_path_text_pattern_ops is used to improve performance of LIKE statements for module_path. It is used to fetch directories matching a given module_path prefix.';


--
-- Name: idx_modules_series_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_modules_series_path ON public.modules USING btree (series_path);


--
-- Name: idx_modules_sort_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_modules_sort_version ON public.modules USING btree (sort_version DESC, version_type DESC);


--
-- Name: INDEX idx_modules_sort_version; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_modules_sort_version IS 'INDEX idx_versions_semver_sort is used to sort versions in order of descending latest. It is used to get the latest version of a package/module and to fetch all versions of a package/module in semver order.';


--
-- Name: idx_modules_version_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_modules_version_type ON public.modules USING btree (version_type);


--
-- Name: INDEX idx_modules_version_type; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_modules_version_type IS 'INDEX idx_versions_version_type is used when fetching versions for a given version_type.';


--
-- Name: idx_mvs_unprocessed_timestamp; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_mvs_unprocessed_timestamp ON public.module_version_states USING btree (index_timestamp) WHERE (last_processed_at IS NULL);


--
-- Name: idx_new_symbol_history_goarch; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_goarch ON public.symbol_history USING btree (goarch);


--
-- Name: idx_new_symbol_history_goos; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_goos ON public.symbol_history USING btree (goos);


--
-- Name: idx_new_symbol_history_module_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_module_path_id ON public.symbol_history USING btree (module_path_id);


--
-- Name: idx_new_symbol_history_parent_symbol_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_parent_symbol_name_id ON public.symbol_history USING btree (parent_symbol_name_id);


--
-- Name: idx_new_symbol_history_since_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_since_version ON public.symbol_history USING btree (since_version);


--
-- Name: idx_new_symbol_history_sort_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_sort_version ON public.symbol_history USING btree (sort_version);


--
-- Name: idx_new_symbol_history_symbol_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_new_symbol_history_symbol_name_id ON public.symbol_history USING btree (symbol_name_id);


--
-- Name: idx_package_symbols_module_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_module_path_id ON public.package_symbols USING btree (module_path_id);


--
-- Name: idx_package_symbols_package_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_package_path_id ON public.package_symbols USING btree (package_path_id);


--
-- Name: idx_package_symbols_parent_symbol_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_parent_symbol_name_id ON public.package_symbols USING btree (parent_symbol_name_id);


--
-- Name: idx_package_symbols_section; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_section ON public.package_symbols USING btree (section);


--
-- Name: idx_package_symbols_symbol_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_symbol_name_id ON public.package_symbols USING btree (symbol_name_id);


--
-- Name: idx_package_symbols_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_symbols_type ON public.package_symbols USING btree (type);


--
-- Name: idx_package_version_states_module_path_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_package_version_states_module_path_version ON public.package_version_states USING btree (module_path, version);


--
-- Name: idx_path_documents_tsv_path_tokens; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_path_documents_tsv_path_tokens ON public.search_documents USING gin (tsv_path_tokens);


--
-- Name: idx_search_documents_imported_by_count_updated_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_imported_by_count_updated_at ON public.search_documents USING btree (imported_by_count_updated_at);


--
-- Name: INDEX idx_search_documents_imported_by_count_updated_at; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_search_documents_imported_by_count_updated_at IS 'INDEX idx_search_documents_imported_by_count_updated_at index is used for incremental update of imported_by counts.';


--
-- Name: idx_search_documents_ln_imported_by_count_desc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_ln_imported_by_count_desc ON public.search_documents USING btree (ln_imported_by_count DESC);


--
-- Name: idx_search_documents_module_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_module_path ON public.search_documents USING btree (module_path);


--
-- Name: idx_search_documents_module_path_version_package_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_module_path_version_package_path ON public.search_documents USING btree (package_path, module_path, version);


--
-- Name: INDEX idx_search_documents_module_path_version_package_path; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_search_documents_module_path_version_package_path IS 'INDEX idx_search_documents_module_path_version_package_path is used for the FK reference to packages.';


--
-- Name: idx_search_documents_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_name ON public.search_documents USING btree (name);


--
-- Name: idx_search_documents_package_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_package_path_id ON public.search_documents USING btree (package_path_id);


--
-- Name: idx_search_documents_tsv_search_tokens; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_tsv_search_tokens ON public.search_documents USING gin (tsv_search_tokens);


--
-- Name: INDEX idx_search_documents_tsv_search_tokens; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_search_documents_tsv_search_tokens IS 'INDEX idx_search_documents_tsv_search_tokens improves performance for full-text search.';


--
-- Name: idx_search_documents_unit_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_unit_id ON public.search_documents USING btree (unit_id);


--
-- Name: idx_search_documents_version_updated_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_search_documents_version_updated_at ON public.search_documents USING btree (version_updated_at);


--
-- Name: INDEX idx_search_documents_version_updated_at; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON INDEX public.idx_search_documents_version_updated_at IS 'INDEX idx_search_documents_version_updated_at is used for incremental update of imported_by counts, in order to determine when the latest version of a package was last updated.';


--
-- Name: idx_symbol_names_lowercase_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_names_lowercase_name ON public.symbol_names USING btree (lower(name));


--
-- Name: idx_symbol_search_documents_imported_by_count_desc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_imported_by_count_desc ON public.symbol_search_documents USING btree (imported_by_count DESC);


--
-- Name: idx_symbol_search_documents_ln_imported_by_count_desc; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_ln_imported_by_count_desc ON public.symbol_search_documents USING btree (ln_imported_by_count DESC);


--
-- Name: idx_symbol_search_documents_lowercase_symbol_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_lowercase_symbol_name ON public.symbol_search_documents USING btree (lower(symbol_name));


--
-- Name: idx_symbol_search_documents_package_symbol_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_package_symbol_id ON public.symbol_search_documents USING btree (package_symbol_id);


--
-- Name: idx_symbol_search_documents_symbol_name_imported_by_count; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_symbol_name_imported_by_count ON public.symbol_search_documents USING btree (lower(symbol_name), imported_by_count DESC);


--
-- Name: idx_symbol_search_documents_unit_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_unit_id ON public.symbol_search_documents USING btree (unit_id);


--
-- Name: idx_symbol_search_documents_uuid_package_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_uuid_package_name ON public.symbol_search_documents USING btree (uuid_package_name);


--
-- Name: idx_symbol_search_documents_uuid_package_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbol_search_documents_uuid_package_path ON public.symbol_search_documents USING btree (uuid_package_path);


--
-- Name: idx_symbols_search_documents_symbol_name_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_symbols_search_documents_symbol_name_id ON public.symbol_search_documents USING btree (symbol_name_id);


--
-- Name: idx_units_module_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_units_module_id ON public.units USING btree (module_id);


--
-- Name: idx_units_path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_units_path_id ON public.units USING btree (path_id);


--
-- Name: idx_units_v1path_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_units_v1path_id ON public.units USING btree (v1path_id);


--
-- Name: idx_version_map_module_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_version_map_module_id ON public.version_map USING btree (module_id);


--
-- Name: idx_version_map_module_path; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_version_map_module_path ON public.version_map USING btree (module_path, resolved_version);


--
-- Name: idx_version_map_requested_version_module_path_resolved_version; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_version_map_requested_version_module_path_resolved_version ON public.version_map USING btree (requested_version, module_path, resolved_version);


--
-- Name: package_symbols_package_path_id_module_path_id_symbol_name_id_p; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX package_symbols_package_path_id_module_path_id_symbol_name_id_p ON public.package_symbols USING btree (package_path_id, module_path_id, symbol_name_id, parent_symbol_name_id, public.uuid_generate_v5(public.uuid_nil(), synopsis));


--
-- Name: search_documents_unit_id_key; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX search_documents_unit_id_key ON public.search_documents USING btree (unit_id);


--
-- Name: units_path_id_module_id_key; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX units_path_id_module_id_key ON public.units USING btree (path_id, module_id);


--
-- Name: search_documents set_ln_imported_by_count; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_ln_imported_by_count BEFORE INSERT OR UPDATE ON public.search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_ln_imported_by_count();


--
-- Name: symbol_search_documents set_ln_imported_by_count; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_ln_imported_by_count BEFORE INSERT OR UPDATE ON public.symbol_search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_ln_imported_by_count();


--
-- Name: search_documents set_symbol_search_documents_imported_by_count; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_symbol_search_documents_imported_by_count AFTER INSERT OR UPDATE ON public.search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_symbol_search_documents_imported_by_count();


--
-- Name: latest_module_versions set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.latest_module_versions FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: TRIGGER set_updated_at ON latest_module_versions; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TRIGGER set_updated_at ON public.latest_module_versions IS 'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';


--
-- Name: modules set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.modules FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: TRIGGER set_updated_at ON modules; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TRIGGER set_updated_at ON public.modules IS 'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';


--
-- Name: package_symbols set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.package_symbols FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: package_version_states set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.package_version_states FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: TRIGGER set_updated_at ON package_version_states; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TRIGGER set_updated_at ON public.package_version_states IS 'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';


--
-- Name: paths set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.paths FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: search_documents set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: TRIGGER set_updated_at ON search_documents; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TRIGGER set_updated_at ON public.search_documents IS 'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';


--
-- Name: symbol_names set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.symbol_names FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: symbol_search_documents set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.symbol_search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: units set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.units FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: TRIGGER set_updated_at ON units; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TRIGGER set_updated_at ON public.units IS 'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';


--
-- Name: version_map set_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON public.version_map FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_updated_at();


--
-- Name: symbol_search_documents set_uuid_package_name; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_uuid_package_name BEFORE INSERT ON public.symbol_search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_uuid_package_name();


--
-- Name: symbol_search_documents set_uuid_package_path; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER set_uuid_package_path BEFORE INSERT ON public.symbol_search_documents FOR EACH ROW EXECUTE FUNCTION public.trigger_modify_uuid_package_path();


--
-- Name: documentation documentation_big_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation
    ADD CONSTRAINT documentation_big_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.units(id) ON DELETE CASCADE;


--
-- Name: legacy_documentation_symbols documentation_symbols_documentation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.legacy_documentation_symbols
    ADD CONSTRAINT documentation_symbols_documentation_id_fkey FOREIGN KEY (documentation_id) REFERENCES public.documentation(id) ON DELETE CASCADE;


--
-- Name: documentation_symbols documentation_symbols_documentation_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation_symbols
    ADD CONSTRAINT documentation_symbols_documentation_id_fkey1 FOREIGN KEY (documentation_id) REFERENCES public.documentation(id) ON DELETE CASCADE;


--
-- Name: legacy_documentation_symbols documentation_symbols_package_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.legacy_documentation_symbols
    ADD CONSTRAINT documentation_symbols_package_id_fkey FOREIGN KEY (package_symbol_id) REFERENCES public.package_symbols(id) ON DELETE CASCADE;


--
-- Name: documentation_symbols documentation_symbols_package_symbol_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.documentation_symbols
    ADD CONSTRAINT documentation_symbols_package_symbol_id_fkey FOREIGN KEY (package_symbol_id) REFERENCES public.package_symbols(id) ON DELETE CASCADE;


--
-- Name: imports imports_to_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.imports
    ADD CONSTRAINT imports_to_path_id_fkey FOREIGN KEY (to_path_id) REFERENCES public.paths(id) ON DELETE CASCADE;


--
-- Name: imports imports_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.imports
    ADD CONSTRAINT imports_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.units(id) ON DELETE CASCADE;


--
-- Name: latest_module_versions latest_module_versions_module_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.latest_module_versions
    ADD CONSTRAINT latest_module_versions_module_path_id_fkey FOREIGN KEY (module_path_id) REFERENCES public.paths(id) ON DELETE CASCADE;


--
-- Name: licenses licenses_module_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.licenses
    ADD CONSTRAINT licenses_module_id_fkey FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- Name: symbol_history new_symbol_history_module_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_module_path_id_fkey FOREIGN KEY (module_path_id) REFERENCES public.paths(id) ON DELETE CASCADE;


--
-- Name: symbol_history new_symbol_history_package_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_package_path_id_fkey FOREIGN KEY (package_path_id) REFERENCES public.paths(id) ON DELETE CASCADE;


--
-- Name: symbol_history new_symbol_history_package_symbol_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_package_symbol_id_fkey FOREIGN KEY (package_symbol_id) REFERENCES public.package_symbols(id) ON DELETE CASCADE;


--
-- Name: symbol_history new_symbol_history_parent_symbol_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_parent_symbol_name_id_fkey FOREIGN KEY (parent_symbol_name_id) REFERENCES public.symbol_names(id) ON DELETE CASCADE;


--
-- Name: symbol_history new_symbol_history_symbol_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_history
    ADD CONSTRAINT new_symbol_history_symbol_name_id_fkey FOREIGN KEY (symbol_name_id) REFERENCES public.symbol_names(id) ON DELETE CASCADE;


--
-- Name: package_symbols package_symbols_parent_symbol_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.package_symbols
    ADD CONSTRAINT package_symbols_parent_symbol_name_id_fkey FOREIGN KEY (parent_symbol_name_id) REFERENCES public.symbol_names(id) ON DELETE CASCADE;


--
-- Name: package_symbols package_symbols_symbol_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.package_symbols
    ADD CONSTRAINT package_symbols_symbol_name_id_fkey FOREIGN KEY (symbol_name_id) REFERENCES public.symbol_names(id) ON DELETE CASCADE;


--
-- Name: package_version_states package_version_states_module_path_version_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.package_version_states
    ADD CONSTRAINT package_version_states_module_path_version_fkey FOREIGN KEY (module_path, version) REFERENCES public.module_version_states(module_path, version) ON DELETE CASCADE;


--
-- Name: units paths_module_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units
    ADD CONSTRAINT paths_module_id_fkey FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- Name: readmes readmes_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.readmes
    ADD CONSTRAINT readmes_path_id_fkey FOREIGN KEY (unit_id) REFERENCES public.units(id) ON DELETE CASCADE;


--
-- Name: search_documents search_documents_package_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.search_documents
    ADD CONSTRAINT search_documents_package_path_id_fkey FOREIGN KEY (package_path_id) REFERENCES public.paths(id) ON DELETE CASCADE;


--
-- Name: search_documents search_documents_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.search_documents
    ADD CONSTRAINT search_documents_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.units(id) ON DELETE CASCADE;


--
-- Name: symbol_search_documents symbol_search_documents_package_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_package_path_id_fkey FOREIGN KEY (package_path_id) REFERENCES public.search_documents(package_path_id) ON DELETE CASCADE;


--
-- Name: symbol_search_documents symbol_search_documents_package_symbol_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_package_symbol_id_fkey FOREIGN KEY (package_symbol_id) REFERENCES public.package_symbols(id) ON DELETE CASCADE;


--
-- Name: symbol_search_documents symbol_search_documents_symbol_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_symbol_name_id_fkey FOREIGN KEY (symbol_name_id) REFERENCES public.symbol_names(id) ON DELETE CASCADE;


--
-- Name: symbol_search_documents symbol_search_documents_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.symbol_search_documents
    ADD CONSTRAINT symbol_search_documents_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.units(id) ON DELETE CASCADE;


--
-- Name: units units_path_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units
    ADD CONSTRAINT units_path_id_fkey FOREIGN KEY (path_id) REFERENCES public.paths(id) NOT VALID;


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: postgres
--

REVOKE USAGE ON SCHEMA public FROM PUBLIC;
GRANT ALL ON SCHEMA public TO PUBLIC;


--
-- PostgreSQL database dump complete
--
