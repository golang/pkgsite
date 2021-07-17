# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful docker functions and variables.

docker_cleanup() {
  docker-compose -f devtools/docker/compose.yaml down --remove-orphans
}

docker_error() {
  echo ""
  echo "---------- ERROR: docker-compose db logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs db
  echo ""
  echo "---------- ERROR: docker-compose seeddb logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs seeddb
  echo ""
  echo "---------- ERROR: docker-compose frontend logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs frontend
  echo ""
  echo "---------- ERROR: docker-compose chrome logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs chrome
  echo ""
  echo "---------- ERROR: docker-compose e2e logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs e2e
  cleanup
}
