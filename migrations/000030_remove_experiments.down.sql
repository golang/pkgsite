-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE experiments (
    name text NOT NULL,
    rollout integer DEFAULT 0 NOT NULL,
    description text NOT NULL,
    PRIMARY KEY (name),
    CONSTRAINT experiments_rollout_check CHECK (((rollout >= 0) AND (rollout <= 100)))
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
