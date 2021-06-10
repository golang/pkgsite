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

    go run ./cmd/frontend [-dev] [-direct_proxy]

- The `-dev` flag reloads templates on each page load and rebuilds JavaScript
  assets when TypeScript source files change.

The frontend can use one of three datasources:

- Postgres database
- Proxy service

The `Datasource` interface implementation is available at internal/datasource.go.

You can use the `-direct_proxy` flag to run the frontend with its datasource as
the proxy service. This allows you to run the frontend without setting up a
postgres database.

Alternatively, you can run pkg.go.dev with a local database. See instructions
on how to [set up](postgres.md) and
[populate](worker.md#populating-data-locally-using-the-worker)
your local database with packages of your choice.

You can then run the frontend with: `go run ./cmd/frontend`

If you add, change or remove any inline scripts in templates, run
`devtools/cmd/csphash` to update the hashes. Running `all.bash`
will do that as well.

### Local mode

You can also use run the frontend locally with an in-memory datasource
populated with modules loaded from your local filesystem.

    go run ./cmd/pkgsite [path1,path2]

This allows you to run the frontend without setting up a database and to view
documentation of local modules without requiring a proxy. The command accepts a
list of comma-separated strings each representing a path of a module to load
into memory.

### End-to-End (E2E) Tests

In addition to tests written in Go inside internal/frontend and
internal/testing/integration, pages on pkg.go.dev may have accessibility tree
and image snapshot tests. These tests will create diffs for inspection on
failure. Timeouts and diff thresholds are configurable for image snapshots if
adjustments are needed to prevent test flakiness. See the
[API](https://github.com/americanexpress/jest-image-snapshot#%EF%B8%8F-api) for
jest image snapshots for more information.

These tests are in the [e2e/ directory](../e2e). For details, see
[e2e/README.md](../e2e/README.md).

## Static Assets

JavaScript assets for pkg.go.dev are compiled from TypeScript files in the
content/static/js directory. The compiled assets are commited to the repo so the
frontend or worker service can be run without any additional steps from a new
clone of the project.

### Building

When modifying any TypeScript code, you must run
`go run ./devtools/cmd/static` before commiting your changes.

### Testing

You can test html and static asset changes by running `npm test`.
This will run the TypeScript type checker and unit tests.

### Linting

Lint your changes by running `npm run lint`. This will run stylelint
and eslint on CSS and TS files in content/static. You can autofix some errors by
running `npm run lint -- --fix`.

### Running npm commands with docker

To run the the unit tests or linters without installing npm prefix the
command with `./all.bash`. This will run the npm through a docker
container that has the pkgsite code mounted in an internal directory.

`./all.bash npm run <command>`
