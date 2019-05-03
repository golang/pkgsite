#!/usr/bin/env bash
# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

if [ -t 1 ] && which tput >/dev/null 2>&1; then
  RED="$(tput setaf 1)"
  GREEN="$(tput setaf 2)"
  YELLOW="$(tput setaf 3)"
  NORMAL="$(tput sgr0)"
else
  RED=""
  GREEN=""
  YELLOW=""
  NORMAL=""
fi

info() { echo -e "${GREEN}$@${NORMAL}" 1>&2; }
warn() { echo -e "${YELLOW}$@${NORMAL}" 1>&2; }
err() { echo -e "${RED}$@${NORMAL}" 1>&2; }

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

checkheaders() {
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

findcode() {
  find ${codedirs[@]} \
    -not -path 'internal/thirdparty/*' \
    \( -name *.go -o -name *.sql -o -name *.tmpl -o -name *.css \)
}

# Check that all .go and .sql files that have been staged in this commit have a
# license header.
info "Running: Checking staged files for license header"
checkheaders $(git diff --cached --name-only | grep -E ".go$|.sql$")
info "Running: Checking internal files for license header"
checkheaders $(findcode)

# Download staticcheck if it doesn't exist
if ! [ -x "$(command -v staticcheck)" ]; then
  info "Running: go get -u honnef.co/go/tools/cmd/staticcheck"
  go get -u honnef.co/go/tools/cmd/staticcheck
fi

info "Running: staticcheck ./... (skipping thirdparty)"
staticcheck $(go list ./... | grep -v thirdparty) | warnout

# Download misspell if it doesn't exist
if ! [ -x "$(command -v misspell)" ]; then
  info "Running: go get -u github.com/client9/misspell/cmd/misspell"
  go get -u github.com/client9/misspell/cmd/misspell
fi

info "Running: misspell cmd/**/* internal/**/* README.md"
misspell cmd/**/* internal/**/* README.md | warnout

info "Running: go mod tidy"
go mod tidy

info "Running: go test ./..."
go test -count=1 ./...

# This test needs to be run separately since an attempt to use the given flag
# will fail if other tests caught by "./..." don't have it defined.
info "Running: go test ./internal/secrets -use_cloud"
go test ./internal/secrets -use_cloud
