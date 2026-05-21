# pkgsite-cli

A command-line interface for querying [pkg.go.dev](https://pkg.go.dev/).

Currently, the API is on `v1beta`, but we expect to move to `v1` soon.

Related to issue [76718](https://go.dev/issue/76718).

## Quick start

To install the `pkgsite-cli` tool, run:

```bash
go install golang.org/x/pkgsite/cmd/internal/pkgsite-cli@latest
```

## Motivation

The [pkg.go.dev](https://pkg.go.dev/) service provides an API interface at
https://pkg.go.dev/api to allow querying information about published Go
packages and modules. The API uses a stateless, GET-only architecture designed
for stability and efficient caching. `pkgsite-cli` is a lightweight CLI that
uses this API. There is no official SDK for the API, but this tool serves as a
reference client implementation that developers can use in other projects. See
the [API spec](https://pkg.go.dev/api) and the
[OpenAPI specification](https://pkg.go.dev/v1beta/openapi.yaml).

## Relationship to existing tools

- **`go doc`** renders documentation for packages available locally.
  `pkgsite-cli` does not replace it for reading local documentation.
- **`cmd/pkgsite`** is a web server that serves documentation for packages
  available locally. It does not provide full version listings, vulnerability
  reports, reverse dependencies, licenses, or search capabilities (yet).
- **`pkgsite-cli`** provides access to information that `go doc` or a local
  instance of `cmd/pkgsite` cannot reach: version listings, vulnerability
  reports, reverse dependencies, licenses, documentation of modules/packages,
  and search results for packages not yet downloaded.

Rule of thumb: Use `go doc` for local code; use `pkgsite-cli` for package
discovery and metadata lookup.

## Commands

Run `pkgsite-cli <command> -h` for details on available flags for each command.

Available commands:
 * `package`
 * `module`
 * `search`

Additional commands will be added in the future.

## Usage Examples

### Search for packages:

```bash
pkgsite-cli search uuid
```

### Inspect a specific package:

```bash
pkgsite-cli package github.com/google/go-cmp/cmp
```

### See reverse dependencies for a package:

```bash
pkgsite-cli package -imported-by github.com/google/go-cmp/cmp
```

### List exported symbols declared by a package:

```bash
pkgsite-cli package -symbols github.com/google/go-cmp/cmp
```

### List versions of a module:

```bash
pkgsite-cli module -versions github.com/google/go-cmp
```

### List both versions and packages belonging to a module:

```bash
pkgsite-cli module -packages -versions github.com/google/go-cmp
```

## Details
- **Ambiguous paths**: Unlike `go mod tidy` or the
  [pkg.go.dev](https://pkg.go.dev) web interface, which use the "longest
  module path" rule to resolve ambiguous package paths, the API requires the
  module to be specified unambiguously. If a package path is ambiguous
  because it exists in multiple modules, the API returns a list of candidates
  and reports an error. Use the `-module` flag to specify the correct
  module path.


## Status and Implementation
- **Experimental**: This tool is currently a prototype.
- **Minimal Dependencies**: To facilitate potential migration to other
  repositories (e.g., `x/tools`), the tool depends only on the Go standard
  library.
- **Duplicate Types**: API request and response types are duplicated in the
  tool's source instead of imported from `pkgsite` for now. We ruled out
  releasing a full SDK because the REST API is simple enough to consume
  directly. This keeps the tool self-contained. However, if this tool remains
  in this repository, we can eliminate this duplication by using the internal
  package.

