# Deploy

The scripts in this directory are meant to be run as steps in Cloud Build.

See the [Cloud Build triggers page](https://pantheon.corp.google.com/cloud-build/triggers?project=go-discovery)
for a list of available triggers.

`deploy-env.yaml` deploys the config, worker, and frontend for a single
environment. The environment is configurable from the Cloud Build trigger
execution page.

`deploy.yaml` deploys to staging, runs e2e tests, and if the tests pass,
deploys to production.

## Go Version Management

The Go runtime version used for deployments and docker environments is managed independently from the `go.mod` language version constraint.

### Cloud Build Configuration

The Go image version is parameterized in all Cloud Build files
(`deploy/*.yaml`) using a `_GO_VERSION` substitution:

```yaml
substitutions:
  _GO_VERSION: 1.27rc1
```

This version is used by all Go build steps via `golang:$_GO_VERSION`.
To update the version for GCP Cloud Build deployments,
update `_GO_VERSION` in the `substitutions:` block of:

- `deploy/deploy.yaml`
- `deploy/deploy-env.yaml`
- `deploy/migrate.yaml`
- `deploy/sitemap.yaml`

### Container Images (Docker builds)

When building service container images (`frontend` and `worker`),
`deploy/deploy-env.yaml` and `deploy/deploy.yaml` pass `$_GO_VERSION`
as an argument to `deploy/build.sh`:

```yaml
- deploy/build.sh $PROJECT_ID $(cat _BUILD_TAG) $_GO_VERSION
```

`deploy/build.sh` forwards this version to `docker build`
via `--build-arg GO_VERSION=$_GO_VERSION`, setting the base image tag
in the private repository's Dockerfiles
(`config/Dockerfile.frontend` and `config/Dockerfile.worker`).

### Docker Compose

For local development and testing under docker,
the Go version is defined in `devtools/docker.sh`:

```bash
export GO_VERSION=${GO_VERSION:-$(get_go_version || echo 1.26.4)}
```

Docker Compose configuration (`devtools/docker/compose.yaml`)
references the `${GO_VERSION}` environment variable.

During Cloud Build execution, the `all.bash` and `screentest` steps explicitly pass
`GO_VERSION=$_GO_VERSION` to the container environment
to ensure testing matches the Cloud Build deployment environment:

```yaml
env:
  - GO_VERSION=$_GO_VERSION
```
