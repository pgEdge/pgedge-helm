# pgedge

pgEdge is fully distributed PostgreSQL with multi-active replication. This chart installs the
community-licensed version of pgEdge as a StatefulSet.

## Commonly used values

Most users will want to specify a database name and user names:

```yaml
pgEdge:
  dbSpec:
    dbName: my_application
    users:
      - username: my_application
        superuser: false
        service: postgres
        type: application
      - username: admin
        # Note this user will only have a subset of superuser privileges to exclude the abilities to
        # read files and execute programs on the host.
        superuser: true
        service: postgres
        type: admin
```

## Examples

See the [examples README](./examples/README.md) for instructions to try this chart using local
Kubernetes clusters created with [`kind`](https://kind.sigs.k8s.io/).

![Version: 0.0.1](https://img.shields.io/badge/Version-0.0.1-informational?style=flat-square)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| annotations | object | `{}` | Additional annotations to apply to all created objects. |
| global.clusterDomain | string | `"cluster.local"` | Set to the cluster's domain if the cluster uses a custom domain. |
| labels | object | `{}` | Additional labels to apply to all created objects. |
| pgEdge.appName | string | `"pgedge"` | Determines the name of the pgEdge StatefulSet and theapp.kubernetes.io/name label. Many other values are derived from this name, so it must be less than or equal to 26 characters in length. |
| pgEdge.dbSpec.dbName | string | `"defaultdb"` | The name of the database to create. |
| pgEdge.dbSpec.nodes | list | `[]` | Used to override the nodes in the generated db spec. This can be useful in multi-cluster setups, like the included multi-cluster example. |
| pgEdge.dbSpec.users | list | `[{"service":"postgres","superuser":false,"type":"application","username":"app"},{"service":"postgres","superuser":true,"type":"admin","username":"admin"}]` | Database users to be created. |
| pgEdge.existingUsersSecret | string | `""` | The name of an existing users secret in the release namespace. If not specified, a new secret will generate random passwords for each user and store them in a new secret. See the pgedge-docker README for the format of this secret: https://github.com/pgEdge/pgedge-docker?tab=readme-ov-file#database-configuration |
| pgEdge.extraMatchLabels | object | `{}` | Specify additional labels to be used in the StatefulSet, Service, and other selectors. |
| pgEdge.imageTag | string | `"kube-testing"` | Set a custom image tag from the docker.io/pgedge/pgedge repository. |
| pgEdge.livenessProbe.enabled | bool | `true` |  |
| pgEdge.livenessProbe.failureThreshold | int | `6` |  |
| pgEdge.livenessProbe.initialDelaySeconds | int | `30` |  |
| pgEdge.livenessProbe.periodSeconds | int | `10` |  |
| pgEdge.livenessProbe.successThreshold | int | `1` |  |
| pgEdge.livenessProbe.timeoutSeconds | int | `5` |  |
| pgEdge.nodeAffinity | object | `{}` |  |
| pgEdge.nodeCount | int | `3` | Sets the number of replicas in the pgEdge StatefulSet. |
| pgEdge.pdb.create | bool | `false` | Enables the creation of a PodDisruptionBudget for pgEdge. |
| pgEdge.pdb.maxUnavailable | string | `""` |  |
| pgEdge.pdb.minAvailable | int | `1` |  |
| pgEdge.podAffinity | object | `{}` |  |
| pgEdge.podAntiAffinityEnabled | bool | `true` | Disable the default pod anti-affinity. By default, this chart uses a preferredDuringSchedulingIgnoredDuringExecution anti-affinity to spread the replicas across different nodes if possible. |
| pgEdge.podAntiAffinityOverride | object | `{}` | Override the default pod anti-affinity. |
| pgEdge.podManagementPolicy | string | `"Parallel"` | Sets how pods are created during the initial scale up. Parallel results in a faster cluster initialization. |
| pgEdge.port | int | `5432` |  |
| pgEdge.readinessProbe.enabled | bool | `true` |  |
| pgEdge.readinessProbe.failureThreshold | int | `6` |  |
| pgEdge.readinessProbe.initialDelaySeconds | int | `5` |  |
| pgEdge.readinessProbe.periodSeconds | int | `5` |  |
| pgEdge.readinessProbe.successThreshold | int | `1` |  |
| pgEdge.readinessProbe.timeoutSeconds | int | `5` |  |
| pgEdge.resources | object | `{}` | Set resource requests and limits. There are none by default. |
| pgEdge.terminationGracePeriodSeconds | int | `10` |  |
| service.annotations | object | `{}` | Additional annotations to apply the the Service. |
| service.clusterIP | string | `""` |  |
| service.externalTrafficPolicy | string | `"Cluster"` |  |
| service.loadBalancerIP | string | `""` |  |
| service.loadBalancerSourceRanges | list | `[]` |  |
| service.name | string | `"pgedge"` | The name of the Service created by this chart. |
| service.sessionAffinity | string | `"None"` |  |
| service.sessionAffinityConfig | object | `{}` |  |
| service.type | string | `"ClusterIP"` |  |
| storage.accessModes[0] | string | `"ReadWriteOnce"` |  |
| storage.annotations | object | `{}` |  |
| storage.className | string | `"standard"` |  |
| storage.labels | object | `{}` |  |
| storage.retentionPolicy.enabled | bool | `false` |  |
| storage.retentionPolicy.whenDeleted | string | `"Retain"` |  |
| storage.retentionPolicy.whenScaled | string | `"Retain"` |  |
| storage.selector | object | `{}` |  |
| storage.size | string | `"8Gi"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.12.0](https://github.com/norwoodj/helm-docs/releases/v1.12.0)
