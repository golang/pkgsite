# pkgsite-cli

A command-line interface for querying pkg.go.dev.

Related to https://go.dev/issue/76718.

## Quick start

To install the `pkgsite-cli` tool, run:

```bash
go install golang.org/x/pkgsite/cmd/internal/pkgsite-cli@latest
```

## Motivation

The pkg.go.dev service exposes a v1 REST API for package and module metadata.
(TODO: link to the API doc).
`pkgsite-cli` provides a lightweight CLI that queries the API and
prints results for both humans and automated tools.
There is no official SDK for the API, but this tool serves as a reference
client implementation that developers can use in other projects.

## Relationship to existing tools

- **`go doc`** renders documentation for packages available locally.
  `pkgsite-cli` does not replace it for reading local documentation.
- **`cmd/pkgsite`** is a webserver that serves documentation for packages
  available locally. It does not provide full version listings, vulnerability
  reports, reverse dependencies, licenses, or search capabilities (yet).
- **`pkgsite-cli`** provides access to information `go doc` or a local instance
  of `cmd/pkgsite` cannot reach: version listings, vulnerability reports, reverse
  dependencies, licenses, documentation of modules/packages, and search
  results for packages not yet downloaded.

Rule of thumb: Use `go doc` for local code; use `pkgsite-cli`
for package discovery and metadata lookup.

## Commands

Run `pkgsite-cli <command> -h` for details on available flags for each command.

Available commands:
 * package
 * module
 * search

More commands will be added.

### Package info

`pkgsite-cli package [flags] <package>[@version]`

Example:
```
$ pkgsite-cli package encoding/json
encoding/json (standard library)
  Module:   std
  Version:  go1.24.2 (latest)
```

### Module info
`pkgsite-cli module [flags] <module>[@version]`

### Search
`pkgsite-cli search [flags] <query>`

## Details
- Ambiguous path: CLI shows candidates. Use `--module` to resolve.
- Pagination: JSON returns `nextPageToken`. Use `--token` to continue.

## Status and Implementation
- **Experimental**: This tool is currently a prototype.
- **Minimal Dependencies**: To facilitate potential migration to other repositories
(e.g. `x/tools`), the tool depends only on the Go standard library.
- **Duplicated Types**: API request/response types are duplicated in the tool's source
  instead of imported from `pkgsite` for now.  We ruled out releasing a full SDK
  because the REST API is simple enough to consume directly.
  This keeps the tool self-contained.  If this tool remains in this repo,
  however, we can eliminate this duplication and use the internal package.
