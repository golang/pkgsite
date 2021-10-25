# Deploy

The scripts in this directory are meant to be run as steps in Cloud Build.

See the [Cloud Build triggers page](https://pantheon.corp.google.com/cloud-build/triggers?project=go-discovery)
for a list of available triggers.

`deploy-env.yaml` deploys the config, worker, and frontend for a single
environment. The environment is configurable from the Cloud Build trigger
execution page.
