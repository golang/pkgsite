#!/usr/bin/env bash
# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

 # Clone SOS license detector if it doesn't exist.
if [ ! -d "sos.googlesource.com" ]; then
  echo "Running: git clone sso://sos/sos sos.googlesource.com/sos"
  git clone sso://sos/sos sos.googlesource.com/sos
fi

# Check that all .go and .sql files that have been staged in this commit have a
# license header.
echo "Running: Checking staged files for license header"
STAGED_GO_FILES=$(git diff --cached --name-only | grep -E ".go$|.sql$")
if [[ "$STAGED_GO_FILES" != "" ]]; then
  for FILE in $STAGED_GO_FILES
  do
      line="$(head -1 $FILE)"
      if [[ ! $line == *"The Go Authors. All rights reserved."* ]] &&
       [[ ! $line == "// DO NOT EDIT. This file was copied from" ]]; then
  	    echo "missing license header: $FILE"
      fi
  done
fi

# Download staticcheck if it doesn't exists
if ! [ -x "$(command -v staticcheck)" ]; then
  echo "Running: go get -u honnef.co/go/tools/cmd/staticcheck"
  go get -u honnef.co/go/tools/cmd/staticcheck
fi

echo "Running: staticcheck ./..."
staticcheck ./...

# Download misspell if it doesn't exists
if ! [ -x "$(command -v misspell)" ]; then
  echo "Running: go get -u github.com/client9/misspell/cmd/misspell"
  go get -u github.com/client9/misspell/cmd/misspell
fi

echo "Running: misspell cmd/**/* internal/**/* README.md"
misspell cmd/**/* internal/**/* README.md

echo "Running: go mod tidy"
go mod tidy

echo "Running: go test ./..."
# We use the `-p 1` flag because several tests must be run in serial due to
# their non-hermetic nature (as they interact with a running Postgres instance).
go test -count=1 -p 1 ./...
