-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE module_version_states ADD COLUMN sort_version text;

COMMENT ON COLUMN module_version_states.sort_version IS
'COLUMN sort_version holds the version in a form suitable for use in ORDER BY. The string format is described in internal/version.ForSorting.';

CREATE INDEX idx_module_version_states_sort_version ON module_version_states (sort_version DESC);

COMMENT ON INDEX idx_module_version_states_sort_version IS
'INDEX idx_module_version_states_sort_version is used to sort by version, to determine when a module version should be retried for processing.';



ALTER TABLE versions ADD COLUMN sort_version text;

COMMENT ON COLUMN versions.sort_version IS
'COLUMN sort_version holds the version in a form suitable for use in ORDER BY.';

CREATE INDEX idx_versions_sort_version ON versions (sort_version DESC, version_type DESC);

COMMENT ON INDEX idx_versions_sort_version IS
'INDEX idx_versions_semver_sort is used to sort versions in order of descending latest. It is used to get the latest version of a package/module and to fetch all versions of a package/module in semver order.';

END;
