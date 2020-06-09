# Contributing to pkg.go.dev

Pkg.go.dev is part of the Go open source project. We would love to receive your
contributions!

Since we are actively working on the site, we ask that you
[file an issue](https://golang.org/s/discovery-feedback) and claim it before
starting to work on something. Otherwise, it is likely that we might already be
working on a fix for your issue.

Because we are currently working on
[design updates to pkg.go.dev](/README.md#roadmap), we will not be accepting
any contributions for
[issues with a UX label](https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+label%3Ago.dev+label%3AUX).

## Finding issues

All issues are labeled with the
[`go.dev` label](https://github.com/golang/go/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+label%3Ago.dev).
Issues that are suitable for contributors are additionally tagged with the
[`help wanted` label](https://github.com/golang/go/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+label%3Ago.dev+label%3A%22help+wanted%22+).

Before you begin working on an issue, please leave a comment that you are claiming it.

## Getting started

1. Complete the steps in the
[Go Contribution Guide](https://golang.org/doc/contribute.html).

2. Download the source code for x/pkgsite:
`git clone https://go.googlesource.com/pkgsite`

3. Review the [design document](doc/design.md).

### Running pkg.go.dev locally

There are two ways to run pkg.go.dev locally.

1. Use a proxy service as a datasource.

2. Use postgres as the datasource.

See [doc/frontend.md](doc/frontend.md) for details.

## Questions

You can find us in the #tools channel on the Gophers Slack.
