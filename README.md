# Go Module Discovery Site

## Getting Started

### Requirements

- [PostgreSQL](https://www.postgresql.org/download/)

### Migrations

Migrations are managed with the [golang-migrate/migrate](github.com/golang-migrate/migrate) [CLI tool](https://github.com/golang-migrate/migrate/tree/master/cli).

To run all the migrations:

```
migrate -source file:migrations -database "postgres://localhost:5432/discovery-database?sslmode=disable" up
```

To create a new migration:

```
migrate create -ext sql -dir migrations -seq <title>
```

This creates two empty files in `/migrations`:

```
{version}_{title}.up.sql
{version}_{title}.down.sql
```

The two migration files are used to migrate "up" to the specified version from the previous version, and to migrate "down" to the previous version. See [golang-migrate/migrate/MIGRATIONS.md](https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md) for details.
