# Multi-cluster Deployments

A single Kubernetes cluster is most commonly deployed in one region, with support for running workloads across multiple availability zones. Most customers who are taking advantage of pgEdge Distributed Postgres operate nodes in different regions for performance or availability reasons, sometimes across multiple Cloud providers.

Deploying across multiple Kubernetes clusters with pgEdge Distributed requires addressing two aspects:

**Network Connectivity:** We must ensure that pgEdge nodes can connect across Kubernetes clusters with cross-cluster DNS using tools such as [Cilium](https://cilium.io/) or [Submariner](https://submariner.io/).

**Certificate Management:** We must ensure that managed users have consistent client certificates across all pgEdge nodes by copying certificates across clusters using different tools.

These domains are well known in the Kubernetes community as part of operating other multi-cluster workloads, and customers often have solutions in place to manage them, so building a single approach into the pgEdge Helm chart doesn’t make sense.

Instead, the new chart includes a few configuration mechanisms to support multi-cluster deployments:

1. `pgEdge.initSpock` - controls whether Spock configuration should be created and updated when deploying the chart. Defaults to true
2. `pgEdge.provisionCerts` - controls whether or not cert-manager certs should be deployed when deploying the chart. Defaults to true
3. `pgEdge.externalNodes` - allows configuring nodes that are part of the pgEdge Distributed Postgres deployment, but managed externally to this Helm chart. These nodes will be configured in the spock-init job when it runs.
4. `internalHostname` - an optional property on each node that specifies a cluster-internal hostname for connectivity checks. This is useful when `hostname` is an external IP or DNS name that cannot be routed from within the Kubernetes cluster where the init-spock job runs.

In order to apply these to a multi-cluster scenario, you can utilize these configuration elements across deployments in multiple clusters.

For example, let’s assume you want to deploy 2 pgEdge nodes across 2 Kubernetes clusters, with a single helm install run against each cluster. These values files highlight how to leverage these options, ensuring that:

- Certificates are only issued during deployment to the first cluster.
- Spock configuration is applied across nodes in both clusters by the initialization job run in the second cluster.

**Cluster A: cluster-a.yaml**

```yaml
pgEdge:
  appName: pgedge
  initSpock: false
  provisionCerts: true
  nodes:
    - name: n1
      hostname: n1.example.com           # External hostname for replication
      internalHostname: pgedge-n1-rw     # Cluster-local service for connectivity checks
      clusterSpec:
        instances: 3
        postgresql:
          synchronous:
            method: any
            number: 1
            dataDurability: required
  externalNodes:
    - name: n2
      hostname: n2.example.com
  clusterSpec:
    storage:
      size: 1Gi
```

**Cluster B: cluster-b.yaml**

```yaml
pgEdge:
  appName: pgedge
  initSpock: true
  provisionCerts: false
  nodes:
    - name: n2
      hostname: n2.example.com           # External hostname for replication
      internalHostname: pgedge-n2-rw     # Cluster-local service for connectivity checks
      clusterSpec:
        instances: 3
        postgresql:
          synchronous:
            method: any
            number: 1
            dataDurability: required
  externalNodes:
    - name: n1
      hostname: n1.example.com
  clusterSpec:
    storage:
      size: 1Gi
```

In this example, each node uses an external hostname (e.g., `n1.example.com`) for the replication DSN that other nodes use to connect, while the `internalHostname` points to the cluster-local Kubernetes service (`pgedge-n1-rw`) that the init-spock job uses to verify the node is ready before configuring replication.

!!! note

    Before deploying Cluster B, the Kubernetes secrets which contain certificates that were issued during Cluster A's deployment must be copied to the new cluster using `kubectl` or another certificate deployment tool.

    ```shell
    kubectl get secrets

    NAME                 TYPE                DATA   AGE
    admin-client-cert    kubernetes.io/tls   3      3m43s
    app-client-cert      kubernetes.io/tls   3      3m43s
    client-ca-key-pair   kubernetes.io/tls   3      3m46s
    pgedge-client-cert   kubernetes.io/tls   3      3m45s
    ```
    
This example assumes you have a cross-cluster DNS solution in place. If you want to simulate this type of deployment in a single Kubernetes cluster, deploying into two separate namespaces should provide a similar experience without needing to handle this aspect.
