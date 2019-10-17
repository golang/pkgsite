-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TYPE version_source AS ENUM (
	'frontend',
	'legal',
	'proxy-index'
);

CREATE TABLE public.documents (
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    package_path text COLLATE pg_catalog."C" NOT NULL,
    module_path text COLLATE pg_catalog."C" NOT NULL,
    series_path text COLLATE pg_catalog."C" NOT NULL,
    package_suffix text NOT NULL,
    version text NOT NULL,
    tsv_search_tokens tsvector
);

CREATE MATERIALIZED VIEW public.mvw_search_documents AS
 SELECT p.path AS package_path,
    p.version,
    p.module_path,
    COALESCE(i.num_imported_by, (0)::bigint) AS num_imported_by,
    p.name,
    v.commit_time,
    p.synopsis,
    p.license_types,
    d.tsv_search_tokens
   FROM (((public.packages p
     JOIN public.documents d ON (((d.module_path = p.module_path) AND (d.version = p.version) AND (d.package_path = p.path))))
     JOIN ( SELECT DISTINCT ON (versions.module_path) versions.module_path,
            versions.version,
            versions.readme_contents,
            versions.commit_time
           FROM public.versions
          ORDER BY versions.module_path,
                CASE
                    WHEN (versions.prerelease = '~'::text) THEN 0
                    ELSE 1

                END, versions.major DESC, versions.minor DESC, versions.patch DESC, versions.prerelease DESC) v ON (((v.module_path = p.module_path) AND (v.version = p.version))))
     LEFT JOIN ( SELECT imports.to_path,
            count(DISTINCT imports.from_path) AS num_imported_by
           FROM public.imports
          WHERE (strpos(imports.to_path, imports.from_module_path) = 0)
          GROUP BY imports.to_path) i ON ((i.to_path = p.path)))
  WHERE (p.path !~~ '%/internal%'::text)
  WITH NO DATA;
CREATE UNIQUE INDEX mvw_search_documents_package_path_module_path_version_unique_id ON
	public.mvw_search_documents USING btree (package_path, module_path, version);
CREATE INDEX mvw_search_documents_tsv_search_tokens_idx ON
	public.mvw_search_documents USING gin (tsv_search_tokens);

END;
