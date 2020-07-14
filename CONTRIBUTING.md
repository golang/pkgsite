# Contributing to pkg.go.dev

Pkg.go.dev is part of the Go open source project. We would love to receive your
contributions!

Since we are actively working on the site, we ask that you
[file an issue](https://golang.org/s/pkgsite-feedback) and claim it before
starting to work on something. Otherwise, it is likely that we might already be
working on a fix for your issue.

Because we are currently working on
[design updates to pkg.go.dev](/README.md#roadmap), we will not be accepting
any contributions for
[issues with a UX label](https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+label%3Ago.dev+label%3AUX).

## Finding issues

All issues related to x/pkgsite are labeled with the
[`pkgsite` label](https://github.com/golang/go/issues/labels/pkgsite).

In particular, we would love your help on issues tagged with the
[`help wanted` label](https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+label%3Apkgsite+label%3A%22help+wanted%22+).

Before you begin working on an issue, please leave a comment that you are claiming it.

## Getting started

1. Complete the steps in the
   [Go Contribution Guide](https://golang.org/doc/contribute.html).

2. Download the source code for x/pkgsite:
   `git clone https://go.googlesource.com/pkgsite`

3. Review the [design document](doc/design.md).

4. If you are contributing a CSS change, please review the
   [Go CSS Coding Guidelines](https://github.com/golang/go/wiki/CSSStyleGuide).

### Running pkg.go.dev locally

There are two ways to run pkg.go.dev locally.

1. Use a proxy service as a datasource.

2. Use postgres as the datasource.

See [doc/frontend.md](doc/frontend.md) for details.

## Before sending a CL for review

1. Run `./all.bash` and fix all resulting errors. See
   [doc/precommit.md](doc/precommit.md) for instructions on setting up a
   pre-commit hook.
2. Ensure your commit message is formatted according to
   [Go conventions](http://golang.org/wiki/CommitMessage).

## Tips for Code Review

After addressing code review comments, mark each comment as:

- "Done" if you did it exactly as described
- "Done" with a reply if you did it, but with a variation
- "Ack" if either it wasn't a request for action, or it wasn't necessary to
  take the action
- A reply to continue the discussion.

If a CL is in progress, but you want to push the intermediate state, it is
helpful to mark the CL as “work in progress”. You can do this using the
three-dot menu at top right corner, and clicking `Mark as work in progress`.
Please do this to indicate to the reviewer(s) that a CL isn’t ready for review.

## Questions

If you are interested in contributing and have questions, come talk to us in the
#pkgsite channel on the [Gophers Slack](https://invite.slack.golangbridge.org)!
