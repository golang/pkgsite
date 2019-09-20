#!/usr/bin/env bash
# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

warnout() {
  while read line; do
    warn "$line"
  done
}

# codedirs lists directories that contain discovery code. If they include
# directories containing external code, those directories must be excluded in
# findcode below.
codedirs=(
  "cmd"
  "content"
  "internal"
  "migrations"
)

# verify_header checks that all given files contain the standard header for Go
# projects.
verify_header() {
  if [[ "$@" != "" ]]; then
    for FILE in $@
    do
        # Allow for the copyright header to start on either of the first two
        # lines, to accomodate conventions for CSS and HTML.
        line="$(head -2 $FILE)"
        if [[ ! $line == *"The Go Authors. All rights reserved."* ]] &&
         [[ ! $line == "// DO NOT EDIT. This file was copied from" ]]; then
              err "missing license header: $FILE"
        fi
    done
  fi
}

# findcode finds source files in the repo, skipping third-party source.
findcode() {
  find ${codedirs[@]} \
    -not -path 'internal/thirdparty/*' \
    \( -name *.go -o -name *.sql -o -name *.tmpl -o -name *.css \)
}

# ensure_go_binary verifies that a binary exists in $PATH corresponding to the
# given go-gettable URI. If no such binary exists, it is fetched via `go get`.
ensure_go_binary() {
  local binary=$(basename $1)
  if ! [ -x "$(command -v $binary)" ]; then
    info "Installing: $1"
    # Run in a subshell for convenience, so that we don't have to worry about
    # our PWD.
    (set -x; cd && env GO111MODULE=on go get -u $1)
  fi
}

# check_headers checks that all source files that have been staged in this
# commit, and all other non-third-party files in the repo, have a license
# header.
check_headers() {
  info "Checking staged files for license header"
  verify_header $(git diff --cached --name-only | grep -E ".go$|.sql$")
  info "Checking internal files for license header"
  verify_header $(findcode)
}

# bad_migrations outputs migrations with bad sequence numbers.
bad_migrations() {
  ls migrations | cut -d _ -f 1 | sort | uniq -c | grep -vE '^\s+2 '
}

# check_bad_migrations looks for sql migration files with bad sequence numbers,
# possibly resulting from a bad merge.
check_bad_migrations() {
  info "Checking for bad migrations"
  bad_migrations | while read line
  do
    err "unexpected number of migrations: $line"
  done
}

# check_staticcheck runs staticcheck on source files.
check_staticcheck() {
  ensure_go_binary honnef.co/go/tools/cmd/staticcheck
  runcmd staticcheck $(go list ./... | grep -v thirdparty | grep -v internal/doc | grep -v internal/render) | warnout
}

# check_misspell runs misspell on source files.
check_misspell() {
  ensure_go_binary github.com/client9/misspell/cmd/misspell
  runcmd misspell cmd/**/* internal/**/* README.md | warnout
}

# check_templates runs go-template-lint on template files. Unfortunately it
# doesn't handler the /helpers/ fileglob correctly, so it is too noisy to be
# included in standard checks.
check_templates() {
  ensure_go_binary sourcegraph.com/sourcegraph/go-template-lint
  runcmd go-template-lint \
    -f=internal/frontend/server.go \
    -t=internal/frontend/server.go \
    -td=content/static/html/pages | warnout
}


run_prettier() {
  if ! [ -x "$(command -v prettier)" ]; then
    err "prettier must be installed"
  fi
  runcmd prettier --write content/static/css/*.css
  runcmd prettier --write content/static/js/*.js
}

# run_go_mod_tidy runs `go mod tidy`.
run_go_mod_tidy() {
  runcmd go mod tidy
}

# run_go_test runs our main set of go tests.
run_go_test() {
  runcmd go test -count=1 $@ ./...
}

# run_go_test_secrets runs tests on the internal/secrets package, which must
# be tested independently in order to accept the -use_cloud flag.
run_go_test_secrets() {
  runcmd go test ./internal/secrets -use_cloud
}

standard_linters() {
  check_headers
  check_bad_migrations
  check_staticcheck
  check_misspell
}

run_tests() {
  run_go_test
  run_go_test_secrets
}

usage() {
  cat <<EOUSAGE
Usage: $0 [subcommand]
Available subcommands:
  help        - display this help message
  (empty)     - run all standard checks and tests
  all         - run all checks and tests, including nonstandard
  ci          - run checks and tests suitable for continuous integration
  test        - run go tests
  tidy        - run go mod tidy
  lint        - run all standard linters below:
  headers     - (lint) check source files for the license disclaimer
  migrations  - (lint) check migration sequence numbers
  misspell    - (lint) run misspell on source files
  staticcheck - (lint) run staticcheck on source files
  prettier    - (lint, nonstandard) run prettier on .js and .css files.
  templates   - (lint, nonstandard) run go-template-lint on templates
EOUSAGE
}

main() {
  case "$1" in
    "-h" | "--help" | "help")
      usage
      exit 0
      ;;
    "")
      standard_linters
      run_go_mod_tidy
      run_tests
      ;;
    all)
      standard_linters
      check_templates
      run_prettier
      run_go_mod_tidy
      run_tests
      ;;
    ci)
      # Similar to the no-arg mode, but omit actions that require GCP
      # permissions or that don't test the code.
      standard_linters
      run_go_test -race
      ;;
    test) run_tests ;;
    tidy) run_go_mod_tidy ;;
    lint) all_linters ;;
    headers) check_headers ;;
    migrations) check_migrations ;;
    misspell) check_misspell ;;
    staticcheck) check_staticcheck ;;
    prettier) run_prettier ;;
    templates) check_templates ;;
    *)
      usage
      exit 1
  esac
  if [[ $EXIT_CODE != 0 ]]; then
    err "FAILED; see errors above"
  fi
  exit $EXIT_CODE
}

main $@
