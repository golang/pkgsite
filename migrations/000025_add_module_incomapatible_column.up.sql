-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules ADD COLUMN incompatible boolean;
ALTER TABLE module_version_states ADD COLUMN incompatible boolean;

COMMENT ON COLUMN modules.incompatible IS
'COLUMN incompatible defines whether the the version for the given module is incompatible';
COMMENT ON COLUMN module_version_states.incompatible IS
'COLUMN incompatible defines whether the the version for the given module is incompatible';

CREATE INDEX idx_modules_incompatible on modules (incompatible);
COMMENT ON INDEX idx_modules_incompatible IS
'INDEX idx_modules_incompatible is used to sort versions if they are incompatible';

CREATE INDEX idx_module_version_states_incompatible on module_version_states (incompatible);
COMMENT ON INDEX idx_module_version_states_incompatible IS
'INDEX idx_module_version_states_incompatible is used to sort versions if they are incompatible';

END;
