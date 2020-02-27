## Services

Pkg.go.dev consists of the following services. All are hosted on App Engine Standard Go 1.13.

- frontend
  - Production version at   - Dev version at   - Run locally with `go run cmd/frontend/main.go`. Pass the
    `-reload_templates` flag to reload templates on each page load. Pass the
    -direct_proxy flag to run directly against the proxy (see above).
- etl  ("extract, transform and load")
  - Production version at   - Dev version at   - Run locally with  `go run cmd/etl/main.go`.
