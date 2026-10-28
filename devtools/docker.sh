# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful docker functions and variables.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

if [[ -z $GO_VERSION ]]; then
  GO_VERSION=$(get_go_version) || true
  if [[ -z $GO_VERSION ]]; then
    die "Failed to detect GO_VERSION from deploy/deploy-env.yaml. Please set GO_VERSION explicitly."
  fi
fi
export GO_VERSION

if [[ -n "$GO_BUILDER_NAME" ]]; then
  export GO_DISCOVERY_POSTGRES_FSYNC=off
fi


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
  docker compose -f "$PWD/devtools/docker/compose.yaml" "$@"
}

if ! command -v docker > /dev/null && command -v podman > /dev/null; then
  docker() {
    if [[ "$1" == "compose" ]]; then
      shift
      podman-compose "$@"
    else
      podman "$@"
    fi
  }
  # Work around bugs in podman-compose < 1.0.6 (Debian 12 ships 1.0.3):
  # 'down' lacks --remove-orphans, and 'run' without --no-deps starts all
  # services in compose.yaml when the target service has no dependencies.
  if ! command podman-compose down --help 2>&1 | grep -q -- --remove-orphans; then
    podman-compose() {
      local args=()
      for arg in "$@"; do
        [[ "$arg" == "--remove-orphans" ]] && continue
        args+=("$arg")
        [[ "$arg" == "run" && " $* " =~ \ (go|nodejs)\  ]] && args+=("--no-deps")
      done
      command podman-compose "${args[@]}"
    }
  fi
fi
