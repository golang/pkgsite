#!/usr/bin/env bash
# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

 # Clone SOS license detector if it doesn't exist.
if [ ! -d "sos.googlesource.com" ]; then
  echo "git clone sso://sos/sos sos.googlesource.com/sos"
  git clone sso://sos/sos sos.googlesource.com/sos
fi

# Update and run staticcheck
go get -u honnef.co/go/tools/cmd/staticcheck
staticcheck ./...

# Tidy modfile
go mod tidy

# Run all tests
go test ./...

# Check that all .go and .sql files that have been staged in this commit have a
# license header.
STAGED_GO_FILES=$(git diff --cached --name-only | grep -E ".go$|.sql$")
echo $STAGED_GO_FILES
if [[ "$STAGED_GO_FILES" = "" ]]; then
  exit 0
fi

for FILE in $STAGED_GO_FILES
do
    line="$(head -1 $FILE)"
    if ! [ "$line" == *"The Go Authors. All rights reserved."* ] &&
    	! [ "$line" == "// DO NOT EDIT. This file was copied from" ]; then
	    echo "missing license header: $FILE"
    fi
done
