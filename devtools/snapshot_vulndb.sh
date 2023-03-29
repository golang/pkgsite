#!/usr/bin/env -S bash -e

# Copyright 2022 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

# Script for copying the latest data from vuln.go.dev (in the legacy format)
# for the tests in tests/screentest/testcases.ci.txt.

origin="https://vuln.go.dev"

copyFiles=(
  "github.com/beego/beego.json"
  "github.com/tidwall/gjson.json"
  "golang.org/x/crypto.json"
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
  "aliases.json"
  "index.json"
  "stdlib.json"
  "toolchain.json"
)

OUT_DIR=$(pwd)/tests/screentest/testdata/vulndb

for f in ${copyFiles[@]}; do
  mkdir -p "$OUT_DIR/$(dirname $f)" && curl -L $origin/$f --output $OUT_DIR/$f
done

index="$OUT_DIR/ID/index.json"
mkdir -p "$(dirname $index)"
echo '["GO-2021-0159","GO-2022-0229","GO-2022-0463","GO-2022-0569","GO-2022-0572","GO-2021-0068","GO-2022-0475","GO-2022-0476","GO-2021-0240","GO-2021-0264","GO-2022-0273"]' > $index
