# Fetch Service

The fetch service is hosted on Kubernetes and available at http://146.148.42.168.

A local version can also be run with `go run cmd/fetch/main.go`.

## Deployment

The fetch service can be deployed following the instructions below. The
instructions assume that you have access to the GCP project `go-discovery` and
keys for the service account `fetch-service`.

### To enable authentication credentials for the cluster:

`gcloud container clusters get-credentials go-discovery-fetch-cluster --zone us-central1-a`

### To create a secret:

`kubectl create secret generic cloudsql-db-credentials --from-literal=<key>=<value>`

To connect to the Cloud SQL Proxy Docker image, two secrets are required:

```
# Google service account credentials
kubectl create secret generic cloudsql-instance-credentials --from-file=credentials.json=/path/to/credentials/locally

# PostgreSQL credentials
kubectl create secret generic cloudsql-db-credentials --from-literal=username=fetch --from-literal=password=<password>
```

### To create a new version of fetch for deployment:

1. View a list of docker image tags at http://gcr.io/go-discovery/fetch or by running `gcloud container images list-tags gcr.io/go-discovery/fetch`.
2. Create a docker image with the next incremental tag, following semver: `docker build -t gcr.io/go-discovery/fetch:v1.0.6 -f cmd/fetch/Dockerfile .`
3. Push the docker image to gcr.io/go-discovery/fetch: `docker push gcr.io/go-discovery/fetch:<version>`

### To deploy an existing version of fetch:

1. Edit the image version in `deployment.yaml`.
2. Apply the changes to kubernetes: `kubectl apply -f deployment.yaml`.
3. Verify: `kubectl describe pod <pod>`.

### To view logs:

1. To get the name of the pod: `watch kubectl get pods`.
2. `kubectl logs <pod> <container>`

### To create the kubernetes service

1. [Create a Kubernetes service.](https://cloud.google.com/kubernetes-engine/docs/how-to/exposing-apps#using_kubectl_expose_to_create_a_service)

2. [Use Cloud Console to create a Static IP.](https://cloud.google.com/kubernetes-engine/docs/tutorials/configuring-domain-name-static-ip#step_2a_using_a_service)

3. Update the Kubernetes Service to attach the static IP to the Service.

4. Run `kubectl get services <fetch-svc-name> -o yaml` to get the service.yaml.
