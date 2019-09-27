-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE OR REPLACE FUNCTION hll_hash(TEXT) RETURNS BIGINT AS $$
	-- This is somewhat a hack, since there is no from_hex function in postgres.
	-- Take the first 64 bits of the md5 hash by converting the hexadecimal
	-- string to bitfield, and then bigint.
	SELECT ('x'||substr(md5($1),1,16))::BIT(64)::BIGINT;
$$ language sql;

CREATE OR REPLACE FUNCTION hll_zeros(BIGINT) RETURNS INT AS $$
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
END; $$
language plpgsql;

END;
