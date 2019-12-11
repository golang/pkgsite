-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE experiments (
	name TEXT NOT NULL PRIMARY KEY,
	rollout INTEGER NOT NULL DEFAULT 0 CHECK (rollout >= 0 AND rollout <= 100),
	description TEXT NOT NULL
);

COMMENT ON TABLE experiments IS
'TABLE experiments contains data for running experiments.';

COMMENT ON COLUMN experiments.name IS
'COLUMN name is the name of the experiment.';

COMMENT ON COLUMN experiments.rollout IS
'COLUMN rollout is the percentage of total requests that are included for the experiment.';

COMMENT ON COLUMN experiments.description IS
'COLUMN description describes the experiment.';

END;
