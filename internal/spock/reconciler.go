// internal/spock/reconciler.go
package spock

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

// Compile-time assertion: SpockReconciler implements resource.Reconciler.
var _ resource.Reconciler = (*SpockReconciler)(nil)

// SpockReconciler implements resource.Reconciler for Spock replication resources.
type SpockReconciler struct {
	cfg   *config.Config
	conns map[string]*pgxpool.Pool
}

// NewReconciler creates a SpockReconciler from config and database connections.
func NewReconciler(cfg *config.Config, conns map[string]*pgxpool.Pool) *SpockReconciler {
	return &SpockReconciler{cfg: cfg, conns: conns}
}

func (r *SpockReconciler) ComputeDesired() map[resource.Identifier]resource.Resource {
	return ComputeDesired(r.cfg, r.conns)
}

func (r *SpockReconciler) RefreshActual(ctx context.Context, desired map[resource.Identifier]resource.Resource) (map[resource.Identifier]resource.Resource, error) {
	return RefreshActual(ctx, r.cfg, r.conns, desired)
}
