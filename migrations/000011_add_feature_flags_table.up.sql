-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE feature_flags (
	feature TEXT NOT NULL PRIMARY KEY,
	rollout INTEGER NOT NULL DEFAULT 0 CHECK (rollout >= 0 AND rollout <= 100),
	description TEXT NOT NULL
);

COMMENT ON TABLE feature_flags IS
'TABLE feature_flags contains features and rollouts for experiments.';

COMMENT ON COLUMN feature_flags.feature IS
'COLUMN feature is the name of the feature that corresponds to an experiment.';

COMMENT ON COLUMN feature_flags.rollout IS
'COLUMN rollout is percentage of total requests that are included for the feature.';

COMMENT ON COLUMN feature_flags.description IS
'COLUMN description describes the experiment for the feature.';

END;
