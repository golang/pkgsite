-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- module_version_states tracks the current state of processing module versions.
CREATE TABLE module_version_states (
  -- identifiers
  module_path TEXT NOT NULL,
  version TEXT NOT NULL,

  -- metadata
  index_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

  -- state machine
  status INT,
  error TEXT,
  try_count INT NOT NULL DEFAULT 0,
  last_processed_at TIMESTAMP WITH TIME ZONE,
  next_processed_after TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

  PRIMARY KEY (module_path, version)
);

-- index on all time dimensions used for pagination
CREATE INDEX module_version_states_index_timestamp_idx ON module_version_states(index_timestamp DESC);
CREATE INDEX module_version_states_next_processed_after_idx ON module_version_states(next_processed_after);
CREATE INDEX module_version_states_last_processed_at_idx ON module_version_states(last_processed_at);

END;
