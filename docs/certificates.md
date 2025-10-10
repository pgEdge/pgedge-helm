This chart leverages `cert-manager` to create a self-signed CA and issue client certificates for a set of managed users on each node:

- `app`: a user for application connections
- `admin`: a superuser for administrative purposes
- `pgedge`: used for multi-active replication connections between nodes
- `streaming_replica`: used for physical replication connections between nodes

This makes it easier to get started with pgEdge, but you may want to use your own CA in production.

In that case, you can disable the self-signed CA creation and certificate issuance by setting
`pgEdge.provisionCerts` to `false`, and issuing your own certificates using cert-manager or another tool.

These can then be plugged into your clusterSpec accordingly:

```yaml
pgEdge:
  appName: pgedge
  provisionCerts: false
  nodes:
    - name: n1
      hostname: pgedge-n1-rw       
    - name: n2
      hostname: pgedge-n2-rw
    - name: n3
      hostname: pgedge-n3-rw
  clusterSpec:
    storage:
      size: 1Gi
    certificates:
        clientCASecret: <secret-containing-client-ca>
        replicationTLSSecret: <secret-containing-replication-tls-cert>
```