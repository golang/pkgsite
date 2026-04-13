# pkginfo

A command-line interface for querying pkg.go.dev.
(Related: https://go.dev/issue/76718).

## Motivation

The pkg.go.dev website exposes a v1 JSON API for package metadata, but currently no command-line tool provides access to it. Developers must use a browser or interact directly with the REST API. Similarly, AI coding agents must either scrape web pages or implement custom API clients. `pkginfo` fills this gap by providing a lightweight CLI that queries the API and prints results for both humans and automated tools.

## Relationship to existing tools

- **`go doc`** renders documentation for packages available locally. `pkginfo` does not replace it for reading local documentation.
- **`cmd/pkgsite`** is a local documentation server. Its data source does not support global information like reverse dependencies or full ecosystem search.
- **`pkginfo`** provides access to information `go doc` cannot reach: version listings, vulnerability reports, reverse dependencies, licenses, documentation
of modules/packages, and search results for packages not yet downloaded.

Rule of thumb: Use `go doc` for local code; use `pkginfo` for ecosystem discovery and metadata.

## Commands

### Package info
`pkginfo [flags] <package>[@version]`

Example:
```
$ pkginfo encoding/json
encoding/json (standard library)
  Module:   std
  Version:  go1.24.2 (latest)
```

Flags:
- `--doc`: Render doc.
- `--examples`: Include examples (requires `--doc`).
- `--imports`: List imports.
- `--imported-by`: List reverse dependencies.
- `--symbols`: List exported symbols.
- `--licenses`: Show licenses.
- `--module=<path>`: Disambiguate module.

### Module info
`pkginfo module [flags] <module>[@version]`

Flags:
- `--readme`: Print README.
- `--licenses`: List licenses.
- `--versions`: List versions.
- `--vulns`: List vulnerabilities.
- `--packages`: List packages in module.

### Search
`pkginfo search [flags] <query>`

Flags:
- `--symbol=<name>`: Search for symbol.

## Common Flags
- `--json`: Output structured JSON.
- `--limit=N`: Max results.
- `--server=URL`: API server URL.

## Details
- Ambiguous path: CLI shows candidates. Use `--module` to resolve.
- Pagination: JSON returns `nextPageToken`. Use `--token` to continue.

## Status and Implementation
- **Experimental**: This tool is currently a prototype.
- **Minimal Dependencies**: To facilitate potential migration to other repositories (e.g. `x/tools`), the tool depends only on the Go standard library. If it stays in this repository, we need to plan for the release/tagging policy.
- **Duplicated Types**: API response types are duplicated in the tool's source instead of imported from `pkgsite` for now. We ruled out releasing a full SDK because the REST API is simple enough to consume directly. This keeps the tool self-contained.
