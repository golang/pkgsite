# Worker

The _worker_ populates the database with information about new modules.

For additional information on functionality of the worker, see the
[design document](design.md).

## Development

The main program lives in `cmd/worker`.

### Running

You can run the worker locally like so:

    go run ./cmd/worker

See [experiment.md](experiment.md) for instructions how to enable experiments.

### Populating data locally using the worker

When run locally, the worker uses an in-memory queue. This implementation has
bounded parallelism (configurable via the `-workers` flag) but does not
automatically retry failures.

In order to populate local versions, you can either fetch the version explicitly
(via `http://localhost:8000/fetch/path/to/package/@v/v1.2.3`), or you can visit the
Worker dashboard, and click 'Enqueue from module index'. This will enqueue the
next N versions from the index for processing.

## Bypassing license checks

By default, the worker does not insert readme contents or documentation into the
database if we determine that the module or package is not redistributable,
based on the licenses it finds in the module zip. To bypass the license check,
pass the flag `-bypass_license_check`.
