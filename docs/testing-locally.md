## Using `kind` to test locally

For local testing, you can use [kind](https://kind.sigs.k8s.io) to create a test cluster that runs on your machine. You'll need access to a Docker host, such as Docker Desktop.

Use this command to install `kind` with Homebrew:

```shell
brew install kind
```

To deploy a local Kubernetes cluster, install kind and run this command:

```shell
kind create cluster --config examples/configs/single/kind.yaml
```

Next, set your kubectl config to use the kind cluster:

```shell
kubectl config use-context kind-single --namespace default
```

To tear down the `kind` cluster, run this command:

```shell
kind delete cluster --name single
```
