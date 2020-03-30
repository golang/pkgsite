-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION popular_search(rawquery text, lim integer, off integer) RETURNS SETOF search_result
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
COMMENT ON FUNCTION popular_search(rawquery text, lim integer, off integer) IS
'FUNCTION popular_search is used to generate results for search. It is implemented as a stored function, so that we can use a cursor to scan search documents procedurally, and stop scanning early, whenever our search results are provably correct.';


END;
