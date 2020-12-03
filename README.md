# Pkg.go.dev

## A site for discovering Go packages

Pkg.go.dev is a website for discovering and evaluating Go packages and modules.

You can check it out at [https://pkg.go.dev](https://pkg.go.dev).

## Roadmap

Pkg.go.dev [launched](https://groups.google.com/g/golang-announce/c/OW8bHSryLIc)
in November 2019, and is currently under active development by the Go team.

Here's what we are currently working on:

- Design updates: We have some design changes planned for pkg.go.dev,
  to address
  [UX feedback](https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+label%3Apkgsite+label%3AUX)
  that we have received.
  You can expect a more cohesive search and navigation experience coming soon.
  We plan to share these designs for feedback once they are ready.

- Godoc.org redirect: Longer term, we are working towards
  [redirecting godoc.org traffic](https://blog.golang.org/pkg.go.dev-2020) to
  pkg.go.dev. We know that there are features available on godoc.org that users
  want to see on pkg.go.dev, and we want to ensure that we address these. We’ve
  been keeping track of issues related to redirecting godoc.org traffic on
  [Go issue #39144](https://golang.org/issue/39144).
  These issues will be prioritized in the next few months. We also plan to
  continue improving our license detection algorithm.

- Search improvements: We’ll be improving our search experience based on
  feedback in [Go issue #37810](https://golang.org/issue/37810),
  to make it easier for users to find the dependencies they are looking for and
  make better decisions around which ones to import.

We encourage everyone to begin using [pkg.go.dev](https://pkg.go.dev) today for
all of their needs and to
[file feedback](https://golang.org/s/pkgsite-feedback)! You can redirect
all of your requests from godoc.org to pkg.go.dev, by clicking
`Always use pkg.go.dev` at the top of any page on [godoc.org](https://godoc.org).

## Issues

If you want to report a bug or have a feature suggestion, please first check
the [known issues](https://github.com/golang/go/labels/pkgsite) to see if your
issue is already being discussed. If an issue does not already exist, feel free
to [file an issue](https://golang.org/s/pkgsite-feedback).

For answers to frequently asked questions, see [go.dev/about](https://go.dev/about).

You can also chat with us on the #tools slack channel on the
[Gophers slack](https://invite.slack.golangbridge.org).

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
