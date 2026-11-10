-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE module_downloads (
    module_path_id BIGINT NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    date           DATE NOT NULL,
    downloads      BIGINT NOT NULL,
    unique_ips     BIGINT NOT NULL,
    PRIMARY KEY (module_path_id, date)
) PARTITION BY RANGE (date);

COMMENT ON TABLE module_downloads IS
'TABLE module_downloads contains daily download and unique IP counts per module.';

CREATE TABLE module_downloads_2025 PARTITION OF module_downloads
    FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');

CREATE TABLE module_downloads_2026 PARTITION OF module_downloads
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

CREATE TABLE module_download_states (
    date        DATE NOT NULL PRIMARY KEY,
    status      INTEGER NOT NULL,
    error       TEXT NOT NULL,
    tries       INTEGER DEFAULT 0 NOT NULL,
    updated_at  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);

COMMENT ON TABLE module_download_states IS
'TABLE module_download_states tracks which daily download count files have been processed by the worker.';

COMMENT ON COLUMN module_download_states.status IS
'COLUMN status is an HTTP status code for the result of the operation.';

COMMENT ON COLUMN module_download_states.tries IS
'COLUMN tries is the number of attempts at reading the date.';

CREATE MATERIALIZED VIEW module_downloads_30d AS
WITH latest AS (
    SELECT MAX(date) AS max_date
    FROM module_download_states
    WHERE status = 200
)
SELECT
    md.module_path_id,
    SUM(md.downloads)::bigint AS downloads_30d
FROM module_downloads md
CROSS JOIN latest l
WHERE l.max_date IS NOT NULL
  AND md.date >= (l.max_date - INTERVAL '29 days')::date
  AND md.date <= l.max_date
GROUP BY md.module_path_id
HAVING SUM(md.downloads) > 0;

COMMENT ON MATERIALIZED VIEW module_downloads_30d IS
'MATERIALIZED VIEW module_downloads_30d contains the 30-day total downloads for each module.';

CREATE UNIQUE INDEX idx_module_downloads_30d_module_path_id
    ON module_downloads_30d (module_path_id);

COMMENT ON INDEX idx_module_downloads_30d_module_path_id IS
'INDEX idx_module_downloads_30d_module_path_id uniquely identifies modules in module_downloads_30d for concurrent refresh and lookup.';

CREATE INDEX idx_module_downloads_30d_downloads_desc
    ON module_downloads_30d (downloads_30d DESC);

COMMENT ON INDEX idx_module_downloads_30d_downloads_desc IS
'INDEX idx_module_downloads_30d_downloads_desc orders modules by 30-day download counts descending.';

END;
