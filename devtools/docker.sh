# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful docker functions and variables.

docker_cleanup() {
  if [ "$GO_DISCOVERY_DOCKER_SKIP_CLEANUP" = "true" ]; then
    echo "Skipping docker cleanup because GO_DISCOVERY_DOCKER_SKIP_CLEANUP=true."
    return
  fi
  dockercompose down --remove-orphans
}

docker_error() {
  if [ "$GO_DISCOVERY_DOCKER_SKIP_LOGS" = "true" ]; then
    echo "Skipping docker logs because GO_DISCOVERY_DOCKER_SKIP_LOGS=true."
    return
  fi
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
}

dockercompose() {
  docker compose -f devtools/docker/compose.yaml $@
}
