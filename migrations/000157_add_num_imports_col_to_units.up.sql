-- Copyright 2025 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units ADD COLUMN num_imports INTEGER;

-- Backfill the num_imports column with the count of imports for each unit.
-- This UPDATE uses a subquery to count imports per unit_id from the imports table.
UPDATE units u
SET num_imports = sub.import_count
FROM (
    SELECT unit_id, COUNT(unit_id) AS import_count
    FROM imports
    GROUP BY unit_id
) AS sub
WHERE u.id = sub.unit_id;

-- Set num_imports to 0 for units that have no entries in the imports table.
UPDATE units
SET num_imports = 0
WHERE num_imports IS NULL;

END;
