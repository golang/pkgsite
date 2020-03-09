# ETL

### Populating data locally using the ETL

When deployed on AppEngine, the discovery ETL uses cloud tasks to manage new
version processing. Cloud tasks offers some nice features, such as built-in
rate limiting and retries with exponential backoff.

When run locally, the ETL uses an in-memory queue. This implementation has
bounded parallelism (configurable via the `-workers` flag) but does not
automatically retry failures.

In order to populate local versions, you can either fetch the version explicitly
(via `http://localhost:8000/fetch/path/to/package/@v/v1.2.3`), or you can visit the
ETL dashboard, and click 'Enqueue from module index'.  This will enqueue the
next N versions from the index for processing.
