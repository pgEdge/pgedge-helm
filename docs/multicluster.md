If you are deploying nodes across multiple Kubernetes clusters, you must take additional steps to leverage this chart, since Helm is not designed to manage resources across multiple clusters by default.

This includes:

1. Installing the required operators (CloudNativePG and cert-manager) into each Kubernetes cluster.
2. Creating required certificates for managed users (app, admin, pgedge, streaming_replica) in each Kubernetes cluster.
3. Installing the chart into each Kubernetes cluster separately, with the following configuration:
  a. Setting `pgEdge.initSpock` to `false` to defer spock initialization until a later deployment.
  b. Setting the `pgEdge.nodes` property to only list nodes which should be deployed in a particular cluster.
4. Exposing the read/write services for each node across clusters using Kubernetes network tools like Cilium or Submariner to enable cross-cluster DNS and connectivity.
5. Installing the chart into one of the clusters with all nodes listed under `pgEdge.externalNodes` and `pgEdge.initSpock` set to `true` to initialize spock configuration.