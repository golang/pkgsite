# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful docker functions and variables.

docker_cleanup() {
  dockercompose down --remove-orphans
}

docker_error() {
  echo ""
  echo "---------- ERROR: docker-compose db logs ----------"
  dockercompose logs db
  echo ""
  echo "---------- ERROR: docker-compose seeddb logs ----------"
  dockercompose logs seeddb
  echo ""
  echo "---------- ERROR: docker-compose frontend logs ----------"
  dockercompose logs frontend
  echo ""
  echo "---------- ERROR: docker-compose chrome logs ----------"
  dockercompose logs chrome
  echo ""
  echo "---------- ERROR: docker-compose e2e logs ----------"
  dockercompose logs e2e
  cleanup
}

dockercompose() {
  docker-compose -f devtools/docker/compose.yaml $@
}
