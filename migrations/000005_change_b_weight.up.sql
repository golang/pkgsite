-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Redefine the popular_search function, which is currently unused,
-- to be the same as popular_search_go_mod but with a B weight of 1.

CREATE OR REPLACE FUNCTION popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) RETURNS SETOF search_result
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
COMMENT ON FUNCTION popular_search(rawquery text, lim integer, off integer, redist_factor real, go_mod_factor real) IS
'FUNCTION popular_search is used to generate results for search. It is implemented as a stored function, so that we can use a cursor to scan search documents procedurally, and stop scanning early, whenever our search results are provably correct.';


END;
