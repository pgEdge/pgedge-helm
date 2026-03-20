// internal/resource/executor.go
package resource

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"
)

// Execute runs plan phases sequentially, parallelizing events within each phase.
func Execute(ctx context.Context, phases [][]Event) error {
	for i, phase := range phases {
		slog.Info("executing phase", "phase", i, "events", len(phase))

		g, ctx := errgroup.WithContext(ctx)
		for _, event := range phase {
			e := event
			g.Go(func() error {
				id := e.Resource.Identifier()
				switch e.Action {
				case ActionCreate:
					slog.Info("creating resource", "type", id.Type, "id", id.ID)
					if err := e.Resource.Create(ctx); err != nil {
						return fmt.Errorf("create %s/%s: %w", id.Type, id.ID, err)
					}
				case ActionDelete:
					slog.Info("deleting resource", "type", id.Type, "id", id.ID)
					if err := e.Resource.Delete(ctx); err != nil {
						return fmt.Errorf("delete %s/%s: %w", id.Type, id.ID, err)
					}
				default:
					slog.Error("unsupported action", "action", e.Action, "type", id.Type, "id", id.ID)
					return fmt.Errorf("unsupported action %d for %s/%s", e.Action, id.Type, id.ID)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return fmt.Errorf("phase %d: %w", i, err)
		}
	}
	return nil
}
