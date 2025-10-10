If you wish to uninstall the chart, you can perform a `helm uninstall` using the following command:

```shell
helm uninstall pgedge
```

All resources will be removed, with the exception of secrets which were created to store generated client certificates by `cert-manager`. 

This is a safety mechanism which aligns with cert-manager's default behavior, and ensures that dependent services are not brought down by an accidental update.

If you wish to delete these secrets, you can query them via `kubectl`:

```shell
kubectl get secrets

NAME                 TYPE                DATA   AGE
pgedge-admin-client-cert    kubernetes.io/tls   3      3m43s
pgedge-app-client-cert      kubernetes.io/tls   3      3m43s
pgedge-client-ca-key-pair   kubernetes.io/tls   3      3m46s
pgedge-pgedge-client-cert   kubernetes.io/tls   3      3m45s
```

From there, you can delete each secret using the following command:

`kubectl delete secret <name>`