// internal/resource/resource_test.go
package resource

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// mockResource implements Resource for testing.
type mockResource struct {
	id           Identifier
	deps         []Identifier
	status       Status
	createCalled bool
	deleteCalled bool
	createErr    error
	deleteErr    error
	mu           sync.Mutex
}

func (m *mockResource) Identifier() Identifier      { return m.id }
func (m *mockResource) Dependencies() []Identifier  { return m.deps }
func (m *mockResource) Refresh(context.Context) error { return nil }
func (m *mockResource) Status() Status               { return m.status }
func (m *mockResource) Create(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalled = true
	return m.createErr
}
func (m *mockResource) Delete(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalled = true
	return m.deleteErr
}

func id(typ, name string) Identifier {
	return Identifier{Type: typ, ID: name}
}

func TestPlanFreshInstall(t *testing.T) {
	desired := map[Identifier]Resource{
		id("user", "n1"):  &mockResource{id: id("user", "n1"), status: Status{Exists: false}},
		id("node", "n1"):  &mockResource{id: id("node", "n1"), deps: []Identifier{id("user", "n1")}, status: Status{Exists: false}},
		id("sub", "n1n2"): &mockResource{id: id("sub", "n1n2"), deps: []Identifier{id("node", "n1"), id("node", "n2")}, status: Status{Exists: false}},
		id("node", "n2"):  &mockResource{id: id("node", "n2"), deps: []Identifier{id("user", "n2")}, status: Status{Exists: false}},
		id("user", "n2"):  &mockResource{id: id("user", "n2"), status: Status{Exists: false}},
	}
	actual := map[Identifier]Resource{}

	events := Plan(actual, desired)
	if len(events) == 0 {
		t.Fatal("expected events for fresh install, got none")
	}

	// All should be creates
	for _, phase := range events {
		for _, e := range phase {
			if e.Action != ActionCreate {
				t.Errorf("expected Create, got %v for %v", e.Action, e.Resource.Identifier())
			}
		}
	}
}

func TestPlanNoOp(t *testing.T) {
	r := &mockResource{id: id("node", "n1"), status: Status{Exists: true}}
	desired := map[Identifier]Resource{id("node", "n1"): r}
	actual := map[Identifier]Resource{id("node", "n1"): r}

	events := Plan(actual, desired)
	total := 0
	for _, phase := range events {
		total += len(phase)
	}
	if total != 0 {
		t.Errorf("expected 0 events for no-op, got %d", total)
	}
}

func TestPlanDeleteRemovedResource(t *testing.T) {
	r := &mockResource{id: id("node", "n3"), status: Status{Exists: true}}
	desired := map[Identifier]Resource{}
	actual := map[Identifier]Resource{id("node", "n3"): r}

	events := Plan(actual, desired)
	total := 0
	for _, phase := range events {
		for _, e := range phase {
			total++
			if e.Action != ActionDelete {
				t.Errorf("expected Delete, got %v", e.Action)
			}
		}
	}
	if total != 1 {
		t.Errorf("expected 1 delete event, got %d", total)
	}
}

func TestPlanRecreate(t *testing.T) {
	r := &mockResource{id: id("node", "n1"), status: Status{Exists: true, NeedsRecreate: true, Reason: "wrong identity"}}
	desired := map[Identifier]Resource{id("node", "n1"): r}
	actual := map[Identifier]Resource{id("node", "n1"): r}

	events := Plan(actual, desired)
	actions := []Action{}
	for _, phase := range events {
		for _, e := range phase {
			actions = append(actions, e.Action)
		}
	}
	// Should have Delete then Create
	if len(actions) != 2 || actions[0] != ActionDelete || actions[1] != ActionCreate {
		t.Errorf("expected [Delete, Create] for recreate, got %v", actions)
	}
}

func TestPlanDependencyOrder(t *testing.T) {
	user := &mockResource{id: id("user", "n1"), status: Status{Exists: false}}
	node := &mockResource{id: id("node", "n1"), deps: []Identifier{id("user", "n1")}, status: Status{Exists: false}}

	desired := map[Identifier]Resource{
		id("user", "n1"): user,
		id("node", "n1"): node,
	}
	actual := map[Identifier]Resource{}

	events := Plan(actual, desired)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 phases for dependency ordering, got %d", len(events))
	}

	// Phase 0 should contain user, phase 1 should contain node
	if len(events[0]) == 0 {
		t.Fatalf("expected at least 1 event in phase 0, got 0")
	}
	if events[0][0].Resource.Identifier() != id("user", "n1") {
		t.Errorf("expected user in phase 0, got %v", events[0][0].Resource.Identifier())
	}
	if len(events[1]) == 0 {
		t.Fatalf("expected at least 1 event in phase 1, got 0")
	}
	if events[1][0].Resource.Identifier() != id("node", "n1") {
		t.Errorf("expected node in phase 1, got %v", events[1][0].Resource.Identifier())
	}
}

func TestExecuteRunsAllPhases(t *testing.T) {
	r1 := &mockResource{id: id("user", "n1")}
	r2 := &mockResource{id: id("node", "n1")}

	plan := [][]Event{
		{{Action: ActionCreate, Resource: r1}},
		{{Action: ActionCreate, Resource: r2}},
	}

	err := Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !r1.createCalled {
		t.Error("r1.Create not called")
	}
	if !r2.createCalled {
		t.Error("r2.Create not called")
	}
}

func TestExecuteStopsOnError(t *testing.T) {
	r1 := &mockResource{id: id("user", "n1"), createErr: fmt.Errorf("fail")}
	r2 := &mockResource{id: id("node", "n1")}

	plan := [][]Event{
		{{Action: ActionCreate, Resource: r1}},
		{{Action: ActionCreate, Resource: r2}},
	}

	err := Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error from Execute")
	}
	if r2.createCalled {
		t.Error("r2.Create should not have been called after phase 0 failed")
	}
}
