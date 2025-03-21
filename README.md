# golang.org/x/pkgsite

This repository hosts the source code of the [pkg.go.dev](https://pkg.go.dev) website,
and [`pkgsite`](https://pkg.go.dev/golang.org/x/pkgsite/cmd/pkgsite), a documentation
server program.

[![Go Reference](https://pkg.go.dev/badge/golang.org/x/pkgsite.svg)](https://pkg.go.dev/golang.org/x/pkgsite)

## pkg.go.dev: a site for discovering Go packages

Pkg.go.dev is a website for discovering and evaluating Go packages and modules.

You can check it out at [https://pkg.go.dev](https://pkg.go.dev).

## pkgsite: a documentation server

`pkgsite` program extracts and generates documentation for Go projects.

Example usage:

```
$ go install golang.org/x/pkgsite/cmd/pkgsite@latest
$ cd myproject
$ pkgsite -open .
```

For more information, see the [pkgsite documentation](https://pkg.go.dev/golang.org/x/pkgsite/cmd/pkgsite).

## Requirements

Pkgsite requires Go 1.23 to run.
The last commit that works with Go 1.18 is
9ffe8b928e4fbd3ff7dcf984254629a47f8b6e63.
The last commit that works with Go 1.17 is
4d836c6a652cde92f433967680dfd6171a91ec12.

## Issues

If you want to report a bug or have a feature suggestion, please first check
the [known issues](https://github.com/golang/go/labels/pkgsite) to see if your
issue is already being discussed. If an issue does not already exist, feel free
to [file an issue](https://golang.org/s/pkgsite-feedback).

For answers to frequently asked questions, see [pkg.go.dev/about](https://pkg.go.dev/about).

You can also chat with us on the
[#pkgsite Slack channel](https://gophers.slack.com/archives/C0166L4QGJV) on the
[Gophers Slack](https://invite.slack.golangbridge.org).

## Contributing

We would love your help!

Our canonical Git repository is located at
[go.googlesource.com/pkgsite](https://go.googlesource.com/pkgsite).
There is a mirror of the repository at
[github.com/golang/pkgsite](https://github.com/golang/pkgsite).

To contribute, please read our [contributing guide](CONTRIBUTING.md).

## License

Unless otherwise noted, the Go source files are distributed under the BSD-style
license found in the [LICENSE](LICENSE) file.

## Links

- [Homepage](https://pkg.go.dev)
- [Feedback](https://golang.org/s/pkgsite-feedback)
- [Issue Tracker](https://golang.org/s/pkgsite-issues)
