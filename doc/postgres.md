### Databases

We use [PostgreSQL](https://www.postgresql.org).

#### Local development database

You will want to run PostgreSQL on your machine for local development.
It should use the default Postgres port of 5432.

If you use a Mac, the easiest way to do that is through installing
https://postgresapp.com.

Another option is to use `docker`. The following docker command will start a
server locally, publish the server's default port to the corresponding local
machine port, and set a password for the default database user (named
`postgres`).

```
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=pick_a_secret -e LANG=C postgres
```

(NOTE: If you have already installed postgres on a workstation using `sudo
apt-get install postgres`, you may have a server already running, and the above
docker command will fail because it can't bind the port. At that point you can
set `GO_DISCOVERY_DATABASE_TEST_`XXX environment variables to use your installed
server, or stop the server using `pg_ctl stop` and use docker. The following
assumes docker.)

You should also install a postgres client (for example `psql`).

Once you have Postgres installed, you can setup the database by running
`./devtools/create_local_db.sh`.

#### DB setup for tests

Depending on your environment, you may need to configure the following
environment variables in order to run tests:

- `GO_DISCOVERY_DATABASE_TEST_USER` (default: 'postgres')
- `GO_DISCOVERY_DATABASE_TEST_PASSWORD` (default: '')
- `GO_DISCOVERY_DATABASE_TEST_HOST` (default: 'localhost')

You don't need to create a database for testing; the tests will automatically
create a database for each package, with the name `discovery_{pkg}_test`. For
example, for internal/etl, tests run on the `discovery_etl_test` database.

If you ever run into issues with your test databases and need to reset them, you
can use ./devtools/drop_test_dbs.sh.

Run `./all.bash` to verify your setup.

#### DB setup for local development

Depending on your environment, you may need to configure the following
environment variables for local development:

- `GO_DISCOVERY_DATABASE_USER` (default: 'postgres')
- `GO_DISCOVERY_DATABASE_PASSWORD`  (default: '')
- `GO_DISCOVERY_DATABASE_HOST` (default: 'localhost')
- `GO_DISCOVERY_DATABASE_NAME` (default: 'discovery-database')

See `internal/config/config.go` for details regarding construction of the
database connection string.

You'll also need to ensure that the database specified by
`GO_DISCOVERY_DATABASE_NAME` exists, and permissions to it are granted to
`GO_DISCOVERY_DATABASE_USER`. You can do that by using `psql` to log in to your
local server, and running `CREATE DATABASE "discovery-database"` (or whatever
name you choose).

Once this database exists and these variables are correctly configured, run
`scripts/create_local_db.sh` once to initialize your local database.

Then apply migrations, as described in 'Migrations' below. You will need to do
this each time a new migration is added, to keep your local schema up-to-date.


### Migrations

Migrations are managed with the [golang-migrate/migrate][] [CLI tool][].

[golang-migrate/migrate]: https://github.com/golang-migrate/migrate
[CLI tool]: https://github.com/golang-migrate/migrate/tree/master/cli


#### Creating a migration

To create a new migration:

```
scripts/create_migration.sh <title>
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

#### Applying migrations for local development

Use the `migrate` CLI:

```
migrate -source file:migrations \
        -database "postgres://postgres@$GO_DISCOVERY_DATABASE_HOST:$GO_DISCOVERY_DATABASE_PORT/$GO_DISCOVERY_DATABASE_NAME}?sslmode=disable&password=${GO_DISCOVERY_DATABASE_PASSWORD}" \
        up
```
