// internal/resource/reconciler.go
package resource

import "context"

// Reconciler provides the desired and actual resource state for a domain.
type Reconciler interface {
	ComputeDesired() map[Identifier]Resource
	RefreshActual(ctx context.Context, desired map[Identifier]Resource) (map[Identifier]Resource, error)
}

// Reconcile drives a full reconciliation cycle: compute desired state,
// refresh actual state, plan the diff, and execute it.
func Reconcile(ctx context.Context, r Reconciler) error {
	desired := r.ComputeDesired()
	actual, err := r.RefreshActual(ctx, desired)
	if err != nil {
		return err
	}
	plan := Plan(actual, desired)
	return Execute(ctx, plan)
}
