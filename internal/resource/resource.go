// internal/resource/resource.go
package resource

import "context"

// Identifier uniquely identifies a resource by type and ID.
type Identifier struct {
	Type string
	ID   string
}

// Status represents the inspected state of a resource.
type Status struct {
	Exists        bool
	NeedsRecreate bool
	NeedsUpdate   bool
	Reason        string
}

// Resource is the core abstraction for a managed object.
type Resource interface {
	Identifier() Identifier
	Dependencies() []Identifier
	Refresh(ctx context.Context) error
	Status() Status
	Create(ctx context.Context) error
	Update(ctx context.Context) error
	Delete(ctx context.Context) error
}

// Action describes what to do with a resource.
type Action int

const (
	ActionCreate Action = iota
	ActionUpdate
	ActionDelete
)

// Event pairs an action with the resource it applies to.
type Event struct {
	Action   Action
	Resource Resource
}
