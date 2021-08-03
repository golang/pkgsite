# Postgres

We use [PostgreSQL](https://www.postgresql.org) to store data served on
pkg.go.dev.

For additional information on our architecture, see the
[design document](design.md).

## Local development database

1. Install PostgreSQL (version 11 or higher) on your machine for local
   development. It should use the default Postgres port of 5432.

   If you use a Mac, the easiest way to do that is through installing
   https://postgresapp.com.

   Another option is to use `docker`. The following docker command will start a
   server locally, publish the server's default port to the corresponding local
   machine port, and set a password for the default database user (named
   `postgres`).

   ```
   ./devtools/docker/compose.sh up -d db
   ```

   (NOTE: If you have already installed postgres on a workstation using
   `sudo apt-get install postgres`, you may have a server already running, and
   the above docker command will fail because it can't bind the port. At that
   point you can set `GO_DISCOVERY_DATABASE_`XXX environment variables to
   use your installed server, or stop the server using `pg_ctl stop` and use
   docker. The following assumes docker.)

   You must also install a postgres client (for example `psql`).

   At this point you should have a Postgres server running on your local machine
   at port 5432.

2. Set the following environment variables:

   - `GO_DISCOVERY_DATABASE_USER` (default: postgres)
   - `GO_DISCOVERY_DATABASE_PASSWORD` (default: '')
   - `GO_DISCOVERY_DATABASE_HOST` (default: localhost)
   - `GO_DISCOVERY_DATABASE_NAME` (default: discovery-db)

   See `internal/config/config.go` for details regarding construction of the
   database connection string.

   If you set up using docker in step 1, you will also need to set
   `GO_DISCOVERY_DATABASE_PASSWORD`. See
   [setting up for tests](postgres.md#setting-up-for-tests) below.

3. Once you have Postgres installed, you should create the `discovery-db` database
   by running `devtools/create_local_db.sh`.

   Then apply migrations, as described in 'Migrations' below. You will need to do
   this each time a new migration is added, to keep your local schema up to date.

## Setting up for tests

Tests require a Postgres instance. If you followed the docker setup in step 1 in
[local development database](postgres.md#local-development-database) above,
then you have one.

When running `go test ./...`, database tests will not run if you don't have
postgres running. To run these tests, set `GO_DISCOVERY_TESTDB=true`.
Alternatively, you can run `./all.bash`, which will run the database tests.

Tests use the following environment variables:

- `GO_DISCOVERY_DATABASE_USER` (default: postgres)
- `GO_DISCOVERY_DATABASE_PASSWORD` (default: '')
- `GO_DISCOVERY_DATABASE_HOST` (default: localhost)
- `GO_DISCOVERY_DATABASE_PORT` (default: 5432)

If you followed the instructions for setting up with docker in step 1 of
[local development database](postgres.md#local-development-database) above,
then you only need to set `GO_DISCOVERY_DATABASE_PASSWORD`.

You don't need to create a database for testing; the tests will automatically
create a database for each package, with the name `discovery_{pkg}_test`. For
example, for internal/worker, tests run on the `discovery_worker_test`
database.

If you ever run into issues with your test databases and need to reset them,
you can run `devtools/drop_test_dbs.sh`.

Run `./all.bash` to verify your setup.

## Migrations

Migrations are managed using
[github.com/golang-migrate/migrate](https://github.com/golang-migrate/migrate),
with the [CLI tool](https://github.com/golang-migrate/migrate/tree/master/cli).

If this is your first time using golang-migrate, check out the
[Getting Started guide](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md).

To install the golang-migrate CLI, follow the instructions in the
[migrate CLI README](https://github.com/golang-migrate/migrate/blob/master/cmd/migrate/README.md).

### Creating a migration

To create a new migration:

```
devtools/create_migration.sh <title>
```

This creates two empty files in `/migrations`:

```
{version}_{title}.up.sql
{version}_{title}.down.sql
```

The two migration files are used to migrate "up" to the specified version from
the previous version, and to migrate "down" to the previous version. See
[golang-migrate/migrate/MIGRATIONS.md](https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md)
for details.

If your migration requires that data be transformed, or that all modules must be
reprocessed, explain in the `up.sql` file how to carry out the data migration.

### Applying migrations for local development

Use the `migrate` CLI:

```
devtools/migrate_db.sh [up|down|force|version] {#}
```

If you are migrating for the first time, choose the "up" command.

For additional details, see
[golang-migrate/migrate/GETTING_STARTED.md#run-migrations](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md#run-migrations).
