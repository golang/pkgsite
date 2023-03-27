#!/usr/bin/env -S bash -e

# Copyright 2023 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

# Script for copying the latest data from the v1 schema in
# vuln.go.dev for the tests in tests/screentest/testcases.ci.txt.

origin="https://vuln.go.dev"

copyFiles=(
  "ID/GO-2021-0159.json"
  "ID/GO-2022-0229.json"
  "ID/GO-2022-0463.json"
  "ID/GO-2022-0569.json"
  "ID/GO-2022-0572.json"
  "ID/GO-2021-0068.json"
  "ID/GO-2022-0475.json"
  "ID/GO-2022-0476.json"
  "ID/GO-2021-0240.json"
  "ID/GO-2021-0264.json"
  "ID/GO-2022-0273.json"
)

go install golang.org/x/vulndb/cmd/indexdb@latest

OUT_DIR=$(pwd)/tests/screentest/testdata/vulndb-v1

for f in ${copyFiles[@]}; do
  mkdir -p "$OUT_DIR/$(dirname $f)" && curl -L $origin/$f --output $OUT_DIR/$f
done

vulns="$OUT_DIR/ID"
indexdb -out $OUT_DIR -vulns $vulns