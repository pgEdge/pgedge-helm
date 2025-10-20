# Connecting To Postgres

You can connect to your deployed pgEdge database using several methods, depending on your security and operational requirements.

## Connecting via kubectl

To connect to a specific database node, use the `kubectl cnpg psql` command with the appropriate details for your cluster.

```shell 
kubectl cnpg psql <NODE_NAME> -- -U <USERNAME> <DATABASE_NAME>
```

The full command structure is: 

- **`<NODE_NAME>`**: The name of the pgEdge node you want to connect to. In a three-node cluster, these are typically named `pgedge-n1`, `pgedge-n2`, etc.
- **`--`**: This is a separator that tells `kubectl` to pass the following arguments directly to the `psql` command.
- **`-U <USERNAME>`**: The user account you want to connect with.
  - `app`: The default user for application access.
  - `admin`: The superuser with full administrative privileges.
- **`<DATABASE_NAME>`**: The name of the database you want to connect to. The default application database is `app`.

**Connect as `app` (standard user)**

To connect to the database named `app` on the node `pgedge-n1` using the `app` user, run:

```shell
kubectl cnpg psql pgedge-n1 -- -U app app
```

**Connect as `admin` (superuser)**

To connect to the database named `admin` on the node `pgedge-n1` using the `admin` superuser, run:

```shell
kubectl cnpg psql pgedge-n1 -- -U admin app
```

## Connecting with client certificate authentication

The pgEdge Helm chart creates certificates for managed users as secrets which you can use in your application for secure authentication and encrypted traffic. Unlike password-based authentication *these are identical across all nodes*. To use them, mount the certificate for the user as a volume in your application's pods like this:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: your-application
spec:
  containers:
  - name: your-application
    image: your-application:latest
    volumeMounts:
    - name: app-client-cert
      mountPath: /certificates/app
      readOnly: true
volumes:
  - name: app-client-cert
    secret:
      secretName: app-client-cert
      items:
        - key: tls.crt
          path: tls.crt
          mode: 0600
        - key: tls.key
          path: tls.key
          mode: 0600
        - key: ca.crt
          path: ca.crt
          mode: 0600
```

Then, configure your application to use these certificates when connecting to the Postgres database via a DSN using `sslkey` and `sslcert`.

`host=pgedge-n1-rw dbname=app user=app sslcert=/certificates/app/tls.crt sslkey=/certificates/app/tls.key sslmode=require port=5432`

!!! note
    
    The current version of the pgEdge Helm chart does not implement server certificate verification, so the `sslmode` in your DSN should be set to `require`.

## Connecting with password authentication

While certificate-based authentication is recommended, you may need to connect with a password in certain cases.

By default, the managed `app` user is issued a *unique password for each pgEdge node* which is stored in a Kubernetes secret named `pgedge-n#-app`. You can connect to each node using the following approach of fetching the secret and invoking `psql`.

```shell
kubectl run psql-client --rm -it \
  --image=ghcr.io/pgedge/pgedge-postgres:17-spock5-standard \
  --env "PGPASSWORD=$(kubectl get secret pgedge-n3-app -o jsonpath='{.data.password}' | base64 -d)" \
  -- psql -h pgedge-n3-rw -d app -U app
```
