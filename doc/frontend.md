# Frontend Development


The main program lives in `cmd/frontend`.

You can run the frontend locally like so:
```
go run cmd/frontend/main.go [-reload_templates] [-direct_proxy]
```
- The `-reload_templates` flag reloads templates on each page load.

The frontend can use one of two datasources:

- Postgres database
- proxy service

The `Datasource` interface implementation is available at internal/datasource.go.

The `-direct_proxy` flag can be used to run the frontend with its datasource as
the proxy service.


The bulk of the code lives in `internal/frontend`.
