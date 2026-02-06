# Security

pgEdge Helm is designed to run in security-hardened Kubernetes environments. This guide covers how to deploy in restricted namespaces and customize security contexts.

## Pod Security Standards

Kubernetes [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/) define three security profiles:

- **Privileged**: Unrestricted policy
- **Baseline**: Minimally restrictive, prevents known privilege escalations
- **Restricted**: Heavily restricted, following security best practices

pgEdge Helm's init-spock job and the pgEdge Enterprise Postgres images are configured to comply with the **Restricted** profile by default, allowing deployment into namespaces with strict Pod Security admission controls.

## Default Security Configuration

The init-spock job runs with the following security settings out of the box:

**Pod Security Context:**
```yaml
seccompProfile:
  type: RuntimeDefault
runAsNonRoot: true
fsGroup: 65532
```

All defaults are explicit in `values.yaml` and can be customized or disabled.

**Container Security Context:**
```yaml
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop:
    - ALL
```

These defaults ensure:

- The container runs as a non-root user
- No privilege escalation is possible
- The root filesystem is read-only
- All Linux capabilities are dropped
- Seccomp is enabled with the runtime default profile

## Deploying to Restricted Namespaces

If your namespace has Pod Security admission enabled at the `restricted` level, pgEdge Helm will work without any additional configuration:

```shell
# Namespace with restricted enforcement
kubectl label namespace pgedge pod-security.kubernetes.io/enforce=restricted

# Install as normal
helm install pgedge ./ --values values.yaml
```

## Customizing Security Contexts

You can customize the security contexts if your environment has specific requirements.

### Pod Security Context

Override the pod-level security settings:

```yaml
pgEdge:
  initSpockJobConfig:
    podSecurityContext:
      seccompProfile:
        type: RuntimeDefault
      runAsNonRoot: true
      runAsUser: 1000
      runAsGroup: 1000
      fsGroup: 1000
```

### Container Security Context

Override the container-level security settings:

```yaml
pgEdge:
  initSpockJobConfig:
    containerSecurityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      runAsUser: 1000
      runAsGroup: 1000
      capabilities:
        drop:
          - ALL
```

### Disabling Security Contexts

If you need to disable the security contexts entirely (not recommended for production):

```yaml
pgEdge:
  initSpockJobConfig:
    podSecurityContext: {}
    containerSecurityContext: {}
```

!!! warning "Security Risk"
    Disabling security contexts removes important protections. Only do this for debugging or in development environments where security is not a concern.