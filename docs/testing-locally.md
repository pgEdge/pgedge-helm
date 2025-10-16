# Testing Locally

For local testing, you can use [kind](https://kind.sigs.k8s.io) to create a test cluster that runs on your machine. 

You'll need access to a Docker host, such as [Docker Desktop](https://docs.docker.com/desktop/), with an installed copy of kind.

Refer to the [Installation](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) section of kind's documentation for how to install kind on your system.

## Creating a local kind cluster

Once installed, you can deploy a local Kubernetes cluster, by running this command from the root of pgedge-helm:

```shell
kind create cluster --config examples/configs/single/kind.yaml
```

Once deployed, set your kubectl config to use the kind cluster:

```shell
kubectl config use-context kind-single --namespace default
```

## Removing a local kind cluster

To tear down the `kind` cluster, run this command:

```shell
kind delete cluster --name single
```
