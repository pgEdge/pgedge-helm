// internal/spock/spock_test.go
package spock

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
	"github.com/pgEdge/pgedge-helm/internal/resource"
)

func TestPgEdgeUserIdentifier(t *testing.T) {
	u := NewPgEdgeUser(config.Node{Name: "n1"}, "app", "pgedge", nil)
	id := u.Identifier()
	if id.Type != ResourceTypeUser || id.ID != "n1" {
		t.Errorf("expected spock.user/n1, got %s/%s", id.Type, id.ID)
	}
}

func TestPgEdgeUserNoDependencies(t *testing.T) {
	u := NewPgEdgeUser(config.Node{Name: "n1"}, "app", "pgedge", nil)
	if len(u.Dependencies()) != 0 {
		t.Errorf("PgEdgeUser should have no dependencies, got %v", u.Dependencies())
	}
}

func TestPgEdgeUserInitialStatus(t *testing.T) {
	u := NewPgEdgeUser(config.Node{Name: "n1"}, "app", "pgedge", nil)
	s := u.Status()
	if s.Exists {
		t.Error("initial status should be Exists=false")
	}
}

// Compile-time assertion: PgEdgeUser must implement resource.Resource.
var _ resource.Resource = (*PgEdgeUser)(nil)

func TestSpockNodeIdentifier(t *testing.T) {
	n := NewSpockNode(config.Node{Name: "n1", Hostname: "pgedge-n1-rw"}, "app", "pgedge", nil)
	id := n.Identifier()
	if id.Type != ResourceTypeNode || id.ID != "n1" {
		t.Errorf("expected spock.node/n1, got %s/%s", id.Type, id.ID)
	}
}

func TestSpockNodeDependsOnUser(t *testing.T) {
	n := NewSpockNode(config.Node{Name: "n1"}, "app", "pgedge", nil)
	deps := n.Dependencies()
	if len(deps) != 1 || deps[0].Type != ResourceTypeUser || deps[0].ID != "n1" {
		t.Errorf("expected dep on spock.user/n1, got %v", deps)
	}
}

func TestSpockNodeInitialStatus(t *testing.T) {
	n := NewSpockNode(config.Node{Name: "n1"}, "app", "pgedge", nil)
	s := n.Status()
	if s.Exists {
		t.Error("initial status should be Exists=false")
	}
}

// Compile-time assertion: SpockNode must implement resource.Resource.
var _ resource.Resource = (*SpockNode)(nil)

func TestSubscriptionIdentifier(t *testing.T) {
	src := config.Node{Name: "n1", Hostname: "pgedge-n1-rw"}
	dst := config.Node{Name: "n2", Hostname: "pgedge-n2-rw"}
	s := NewSubscription(src, dst, "app", "pgedge", false, nil)
	id := s.Identifier()
	if id.Type != ResourceTypeSubscription {
		t.Errorf("type: got %q", id.Type)
	}
	if id.ID != "sub_n1_n2" {
		t.Errorf("id: got %q, want sub_n1_n2", id.ID)
	}
}

func TestSubscriptionIdentifierDashesReplaced(t *testing.T) {
	src := config.Node{Name: "us-east-1", Hostname: "pgedge-us-east-1-rw"}
	dst := config.Node{Name: "us-west-2", Hostname: "pgedge-us-west-2-rw"}
	s := NewSubscription(src, dst, "app", "pgedge", false, nil)
	id := s.Identifier()
	if id.ID != "sub_us_east_1_us_west_2" {
		t.Errorf("id: got %q, want sub_us_east_1_us_west_2", id.ID)
	}
}

func TestSubscriptionDependsOnNodesAndSlot(t *testing.T) {
	src := config.Node{Name: "n1"}
	dst := config.Node{Name: "n2"}
	s := NewSubscription(src, dst, "app", "pgedge", false, nil)
	deps := s.Dependencies()
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}
	depMap := map[resource.Identifier]bool{}
	for _, d := range deps {
		depMap[d] = true
	}
	if !depMap[resource.Identifier{Type: ResourceTypeNode, ID: "n1"}] {
		t.Error("missing dependency on spock.node/n1")
	}
	if !depMap[resource.Identifier{Type: ResourceTypeNode, ID: "n2"}] {
		t.Error("missing dependency on spock.node/n2")
	}
	if !depMap[resource.Identifier{Type: ResourceTypeReplicationSlot, ID: "spk_app_n1_sub_n1_n2"}] {
		t.Error("missing dependency on replication slot")
	}
}

func TestSubscriptionSyncFlag(t *testing.T) {
	src := config.Node{Name: "n1"}
	dst := config.Node{
		Name: "n2",
		Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"},
	}
	s := NewSubscription(src, dst, "app", "pgedge", true, nil)
	if !s.sync {
		t.Error("expected sync=true when dst bootstraps from src via spock")
	}

	s2 := NewSubscription(src, dst, "app", "pgedge", false, nil)
	if s2.sync {
		t.Error("expected sync=false when not bootstrapping from this src")
	}
}

// Compile-time assertion: Subscription must implement resource.Resource.
var _ resource.Resource = (*Subscription)(nil)

func TestReplicationSlotIdentifier(t *testing.T) {
	s := NewReplicationSlot("n1", "n2", "app", nil)
	id := s.Identifier()
	if id.Type != ResourceTypeReplicationSlot {
		t.Errorf("type: got %q", id.Type)
	}
	if id.ID != "spk_app_n1_sub_n1_n2" {
		t.Errorf("id: got %q, want spk_app_n1_sub_n1_n2", id.ID)
	}
}

func TestReplicationSlotIdentifierDashesReplaced(t *testing.T) {
	s := NewReplicationSlot("us-east-1", "us-west-2", "my-db", nil)
	id := s.Identifier()
	if id.ID != "spk_my_db_us_east_1_sub_us_east_1_us_west_2" {
		t.Errorf("id: got %q, want spk_my_db_us_east_1_sub_us_east_1_us_west_2", id.ID)
	}
}

func TestReplicationSlotDependsOnProviderNode(t *testing.T) {
	s := NewReplicationSlot("n1", "n2", "app", nil)
	deps := s.Dependencies()
	if len(deps) != 1 || deps[0].Type != ResourceTypeNode || deps[0].ID != "n1" {
		t.Errorf("expected dep on spock.node/n1, got %v", deps)
	}
}

func TestReplicationSlotInitialStatus(t *testing.T) {
	s := NewReplicationSlot("n1", "n2", "app", nil)
	if s.Status().Exists {
		t.Error("initial status should be Exists=false")
	}
}

// Compile-time assertion: ReplicationSlot must implement resource.Resource.
var _ resource.Resource = (*ReplicationSlot)(nil)

func TestComputeDesiredSingleNode(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{{Name: "n1", Hostname: "pgedge-n1-rw"}},
	}
	resources := ComputeDesired(cfg, map[string]*pgxpool.Pool{"n1": nil})

	// 1 user + 1 node + 0 subscriptions
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	if _, ok := resources[resource.Identifier{Type: ResourceTypeUser, ID: "n1"}]; !ok {
		t.Error("missing spock.user/n1")
	}
	if _, ok := resources[resource.Identifier{Type: ResourceTypeNode, ID: "n1"}]; !ok {
		t.Error("missing spock.node/n1")
	}
}

func TestComputeDesiredTwoNodes(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "pgedge-n1-rw"},
			{Name: "n2", Hostname: "pgedge-n2-rw"},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil}
	resources := ComputeDesired(cfg, conns)

	// 2 users + 2 nodes + 2 slots + 2 subscriptions = 8
	if len(resources) != 8 {
		t.Fatalf("expected 8 resources, got %d", len(resources))
	}
	if _, ok := resources[resource.Identifier{Type: ResourceTypeSubscription, ID: "sub_n1_n2"}]; !ok {
		t.Error("missing sub_n1_n2")
	}
	if _, ok := resources[resource.Identifier{Type: ResourceTypeSubscription, ID: "sub_n2_n1"}]; !ok {
		t.Error("missing sub_n2_n1")
	}
}

func TestComputeDesiredThreeNodes(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "h1"},
			{Name: "n2", Hostname: "h2"},
			{Name: "n3", Hostname: "h3"},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil, "n3": nil}
	resources := ComputeDesired(cfg, conns)

	// 3 users + 3 nodes + 6 slots + 6 subscriptions = 18
	if len(resources) != 18 {
		t.Fatalf("expected 18 resources, got %d", len(resources))
	}
}

func TestComputeDesiredSyncFlag(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "h1"},
			{Name: "n2", Hostname: "h2", Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"}},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil}
	resources := ComputeDesired(cfg, conns)

	// sub from n1→n2 should have sync=true (n2 bootstraps from n1)
	rawN1N2, ok := resources[resource.Identifier{Type: ResourceTypeSubscription, ID: "sub_n1_n2"}]
	if !ok {
		t.Fatal("missing sub_n1_n2")
	}
	subN1N2, ok := rawN1N2.(*Subscription)
	if !ok {
		t.Fatalf("sub_n1_n2 has unexpected type %T", rawN1N2)
	}
	if !subN1N2.sync {
		t.Error("sub_n1_n2 should have sync=true")
	}

	// sub from n2→n1 should have sync=false (n1 doesn't bootstrap from n2)
	rawN2N1, ok := resources[resource.Identifier{Type: ResourceTypeSubscription, ID: "sub_n2_n1"}]
	if !ok {
		t.Fatal("missing sub_n2_n1")
	}
	subN2N1, ok := rawN2N1.(*Subscription)
	if !ok {
		t.Fatalf("sub_n2_n1 has unexpected type %T", rawN2N1)
	}
	if subN2N1.sync {
		t.Error("sub_n2_n1 should have sync=false")
	}
}

