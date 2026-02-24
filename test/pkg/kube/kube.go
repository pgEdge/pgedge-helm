package kube

import (
	"fmt"
	"os/exec"
	"strings"
)

// Kubectl wraps the kubectl CLI.
type Kubectl struct {
	Context   string
	Namespace string
}

func (k *Kubectl) baseArgs() []string {
	var args []string
	if k.Context != "" {
		args = append(args, "--context", k.Context)
	}
	if k.Namespace != "" {
		args = append(args, "-n", k.Namespace)
	}
	return args
}

// Get runs kubectl get and returns the output.
func (k *Kubectl) Get(resource, name string) (string, error) {
	args := append(k.baseArgs(), "get", resource, name)
	return k.run(args...)
}

// GetJSON runs kubectl get with -o json.
func (k *Kubectl) GetJSON(resource, name string) ([]byte, error) {
	args := append(k.baseArgs(), "get", resource, name, "-o", "json")
	out, err := k.run(args...)
	return []byte(out), err
}

// Exec runs a command in a pod.
func (k *Kubectl) Exec(pod, container string, cmd ...string) (string, error) {
	args := append(k.baseArgs(), "exec", pod)
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--")
	args = append(args, cmd...)
	return k.run(args...)
}

// ExecSQL runs a SQL query via psql in a pod.
// Uses: kubectl exec <pod> -- psql -U admin -d app -tAc "<sql>"
func (k *Kubectl) ExecSQL(pod, sql string) (string, error) {
	return k.Exec(pod, "postgres", "psql", "-U", "admin", "-d", "app", "-tAc", sql)
}

// Logs returns the logs for a pod.
func (k *Kubectl) Logs(pod string) (string, error) {
	args := append(k.baseArgs(), "logs", pod)
	return k.run(args...)
}

// WaitForCondition runs kubectl wait.
func (k *Kubectl) WaitForCondition(resource, name, condition string, timeout string) error {
	args := append(k.baseArgs(), "wait", resource+"/"+name,
		"--for=condition="+condition, "--timeout="+timeout)
	_, err := k.run(args...)
	return err
}

// WaitForDelete runs kubectl wait --for=delete on a resource.
func (k *Kubectl) WaitForDelete(resource, labelSelector, timeout string) error {
	args := append(k.baseArgs(), "wait", resource,
		"-l", labelSelector, "--for=delete", "--timeout="+timeout)
	_, err := k.run(args...)
	return err
}

// ConnectWithCert creates a temporary pod to connect to a PostgreSQL service
// using TLS client certificate authentication from a Kubernetes secret.
// The secret must contain tls.crt and tls.key.
// The command is specified in the container override to bypass the postgres entrypoint.
func (k *Kubectl) ConnectWithCert(service, certSecret, user, db, sql string) (string, error) {
	podName := fmt.Sprintf("cert-test-%s", user)

	connStr := fmt.Sprintf(
		"host=%s port=5432 dbname=%s user=%s sslmode=require sslcert=/certs/tls.crt sslkey=/certs/tls.key",
		service, db, user,
	)

	overrides := fmt.Sprintf(
		`{"spec":{"volumes":[{"name":"certs","secret":{"secretName":"%s","defaultMode":384}}],"containers":[{"name":"%s","image":"postgres:17","command":["psql","%s","-tAc","%s"],"volumeMounts":[{"name":"certs","mountPath":"/certs","readOnly":true}]}]}}`,
		certSecret, podName, connStr, sql,
	)

	args := append(k.baseArgs(), "run", podName, "--rm", "-i", "--restart=Never",
		"--image=postgres:17", "--overrides", overrides)

	return k.run(args...)
}

func (k *Kubectl) run(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl %s failed: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}
