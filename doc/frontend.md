# Frontend

The _frontend_ presents user-facing web pages on pkg.go.dev.

For additional information on functionality of the frontend, see the
[design document](design.md).

## Development

The main program lives in `cmd/frontend`. The bulk of the code lives in
`internal/frontend`.

See [experiment.md](experiment.md) for instructions how to enable experiments.

### Running

You can run the frontend locally like so:

    go run ./cmd/frontend [-dev] [-direct_proxy] [-local .]

- The `-dev` flag reloads templates on each page load.

The frontend can use one of three datasources:

- Postgres database
- Proxy service
- Local filesystem

The `Datasource` interface implementation is available at internal/datasource.go.

You can use the `-direct_proxy` flag to run the frontend with its datasource as
the proxy service. This allows you to run the frontend without setting up a
postgres database.

Alternatively, you can run pkg.go.dev with a local database. See instructions
on how to [set up](postgres.md) and
[populate](worker.md#populating-data-locally-using-the-worker)
your local database with packages of your choice.

You can then run the frontend with: `go run ./cmd/frontend`

You can also use `-local` flag to run the frontend with an in-memory datasource
populated with modules loaded from your local filesystem. This allows you to run
the frontend without setting up a database and to view documentation of local
modules without requiring a proxy. `-local` accepts a GOPATH-like string containing
paths of modules to load into memory.

If you add, change or remove any inline scripts in templates, run
`devtools/cmd/csphash` to update the hashes. Running `all.bash`
will do that as well.

### Testing

Pages on pkg.go.dev have accessibility tree and image snapshot tests. These tests
will create diffs for inspection on failure. Timeouts and diff thresholds are
configurable for image snapshots if adjustments are needed to prevent test flake.
See the [API](https://github.com/americanexpress/jest-image-snapshot#%EF%B8%8F-api) for jest image snapshots for more information.

To run the tests locally, start the pkgsite server and then run
`./devtools/npm.sh run test-e2e`
### Compiling JS sources

The frontend serves compiled and minified JS. If you're modifying any JS code, make
sure to run `devtools/compile_js.sh` for the changes to take effect. This script will
require Docker to be installed.
