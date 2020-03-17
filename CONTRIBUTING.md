# Contributing to pkg.go.dev

Pkg.go.dev is part of the Go open source project.

We would love to receive your contributions!

Since we are actively working on the site, we ask that you
[file an issue](https://golang.org/s/discovery-feedback) and claim it before
starting to work on something. Otherwise, it is likely that we might already be
working on a fix for your issue.

Please read the [Contribution Guidelines](https://golang.org/doc/contribute.html)
before sending patches.

## Finding issues

All issues are labeled with the [`go.dev`
label](https://github.com/golang/go/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+label%3Ago.dev).
Issues that are suitable for contributors are additionally tagged with the
[`help wanted` label](https://github.com/golang/go/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+label%3Ago.dev+label%3A%22help+wanted%22+).

Before you begin working on an issue, please leave a comment that you are claiming it.


## Getting started

1. Get the source code:

` $ git clone https://go.googlesource.com/discovery`

- Our canonical Git repository is located at [https://go.googlesource.com/discovery](https://go.googlesource.com/discovery). [github.com/golang/discovery](https://github.com/golang/discovery) is a mirror of that repository.

2. Review the [design document](design.md).

3. We deploy to the [Google Cloud Platform](https://cloud.google.com). If you
wish to set up a similar environment, you will want to
download and install the Google Cloud SDK at https://cloud.google.com/sdk/docs/.

4. Depending on the feature you are working on, review the contributing guides for:

- [Frontend development](doc/frontend.md)
- [Worker development](doc/worker.md)
- [Database setup](doc/postgres.md)

## Questions

You can find us in the #tools channel on the Gophers Slack, or you can send us
an email at go-discovery-feedback@google.com.
