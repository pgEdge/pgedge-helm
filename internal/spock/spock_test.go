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
		Name:      "n2",
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

func TestSubscriptionExtraDeps(t *testing.T) {
	src := config.Node{Name: "n1"}
	dst := config.Node{Name: "n2"}
	extra := resource.Identifier{Type: "spock.wait_for_sync_event", ID: "n1_n2"}
	s := NewSubscription(src, dst, "app", "pgedge", false, nil, extra)
	deps := s.Dependencies()
	if len(deps) != 4 {
		t.Fatalf("expected 4 deps (3 base + 1 extra), got %d", len(deps))
	}
	found := false
	for _, d := range deps {
		if d == extra {
			found = true
		}
	}
	if !found {
		t.Errorf("extraDep %v not found in dependencies %v", extra, deps)
	}
}

func TestSubscriptionNoExtraDeps(t *testing.T) {
	src := config.Node{Name: "n1"}
	dst := config.Node{Name: "n2"}
	s := NewSubscription(src, dst, "app", "pgedge", false, nil)
	deps := s.Dependencies()
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps (no extras), got %d", len(deps))
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

// --- ReplicationSlotCreate tests ---

func TestReplicationSlotCreateIdentifier(t *testing.T) {
	r := NewReplicationSlotCreate("n1", "n3", "app", nil)
	id := r.Identifier()
	if id.Type != ResourceTypeReplicationSlotCreate {
		t.Errorf("type: got %q, want %q", id.Type, ResourceTypeReplicationSlotCreate)
	}
	if id.ID != "spk_app_n1_sub_n1_n3" {
		t.Errorf("id: got %q, want spk_app_n1_sub_n1_n3", id.ID)
	}
}

func TestReplicationSlotCreateDependsOnProviderNode(t *testing.T) {
	r := NewReplicationSlotCreate("n1", "n3", "app", nil)
	deps := r.Dependencies()
	if len(deps) != 1 || deps[0].Type != ResourceTypeNode || deps[0].ID != "n1" {
		t.Errorf("expected dep on spock.node/n1, got %v", deps)
	}
}

func TestReplicationSlotCreateEphemeral(t *testing.T) {
	r := NewReplicationSlotCreate("n1", "n3", "app", nil)
	if r.Status().Exists {
		t.Error("ephemeral resource should not exist before refresh")
	}
}

var _ resource.Resource = (*ReplicationSlotCreate)(nil)

// --- SyncEvent tests ---

func TestSyncEventIdentifier(t *testing.T) {
	r := NewSyncEvent("n2", "n1", nil)
	id := r.Identifier()
	if id.Type != ResourceTypeSyncEvent {
		t.Errorf("type: got %q, want %q", id.Type, ResourceTypeSyncEvent)
	}
	if id.ID != "n2_n1" {
		t.Errorf("id: got %q, want n2_n1", id.ID)
	}
}

func TestSyncEventDependencies(t *testing.T) {
	extra := resource.Identifier{Type: ResourceTypeReplicationSlotCreate, ID: "spk_app_n2_sub_n2_n3"}
	r := NewSyncEvent("n2", "n1", nil, extra)
	deps := r.Dependencies()
	// Should depend on node + subscription + extra
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(deps), deps)
	}
}

func TestSyncEventEphemeral(t *testing.T) {
	r := NewSyncEvent("n2", "n1", nil)
	if r.Status().Exists {
		t.Error("ephemeral resource should not exist")
	}
}

var _ resource.Resource = (*SyncEvent)(nil)

// --- WaitForSyncEvent tests ---

func TestWaitForSyncEventIdentifier(t *testing.T) {
	syncEvt := &SyncEvent{providerName: "n1", subscriberName: "n3"}
	r := NewWaitForSyncEvent("n1", "n3", syncEvt, nil)
	id := r.Identifier()
	if id.Type != ResourceTypeWaitForSyncEvent {
		t.Errorf("type: got %q, want %q", id.Type, ResourceTypeWaitForSyncEvent)
	}
	if id.ID != "n1_n3" {
		t.Errorf("id: got %q, want n1_n3", id.ID)
	}
}

func TestWaitForSyncEventDependsOnSyncEvent(t *testing.T) {
	syncEvt := &SyncEvent{providerName: "n1", subscriberName: "n3"}
	r := NewWaitForSyncEvent("n1", "n3", syncEvt, nil)
	deps := r.Dependencies()
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d: %v", len(deps), deps)
	}
	if deps[0].Type != ResourceTypeSyncEvent || deps[0].ID != "n1_n3" {
		t.Errorf("expected dep on sync_event/n1_n3, got %v", deps[0])
	}
}

func TestWaitForSyncEventEphemeral(t *testing.T) {
	r := NewWaitForSyncEvent("n1", "n3", nil, nil)
	if r.Status().Exists {
		t.Error("ephemeral resource should not exist")
	}
}

var _ resource.Resource = (*WaitForSyncEvent)(nil)

// --- LagTrackerCommitTimestamp tests ---

func TestLagTrackerCommitTSIdentifier(t *testing.T) {
	r := NewLagTrackerCommitTimestamp("n2", "n3", nil)
	id := r.Identifier()
	if id.Type != ResourceTypeLagTrackerCommitTS {
		t.Errorf("type: got %q, want %q", id.Type, ResourceTypeLagTrackerCommitTS)
	}
	if id.ID != "n2_n3" {
		t.Errorf("id: got %q, want n2_n3", id.ID)
	}
}

func TestLagTrackerCommitTSDependencies(t *testing.T) {
	extra := resource.Identifier{Type: ResourceTypeWaitForSyncEvent, ID: "n1_n3"}
	r := NewLagTrackerCommitTimestamp("n2", "n3", nil, extra)
	deps := r.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps (node + extra), got %d: %v", len(deps), deps)
	}
}

func TestLagTrackerCommitTSEphemeral(t *testing.T) {
	r := NewLagTrackerCommitTimestamp("n2", "n3", nil)
	if r.Status().Exists {
		t.Error("ephemeral resource should not exist")
	}
}

var _ resource.Resource = (*LagTrackerCommitTimestamp)(nil)

// --- ReplicationSlotAdvanceFromCTS tests ---

func TestReplicationSlotAdvanceIdentifier(t *testing.T) {
	r := NewReplicationSlotAdvanceFromCTS("n2", "n3", "app", nil, nil)
	id := r.Identifier()
	if id.Type != ResourceTypeReplicationSlotAdvanceFromCTS {
		t.Errorf("type: got %q, want %q", id.Type, ResourceTypeReplicationSlotAdvanceFromCTS)
	}
	if id.ID != "n2_n3" {
		t.Errorf("id: got %q, want n2_n3", id.ID)
	}
}

func TestReplicationSlotAdvanceDependsOnNodeAndLagTracker(t *testing.T) {
	lagTracker := &LagTrackerCommitTimestamp{originName: "n2", receiverName: "n3"}
	r := NewReplicationSlotAdvanceFromCTS("n2", "n3", "app", lagTracker, nil)
	deps := r.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps (node + lag tracker), got %d: %v", len(deps), deps)
	}
	if deps[0].Type != ResourceTypeNode || deps[0].ID != "n2" {
		t.Errorf("expected dep on spock.node/n2, got %v", deps[0])
	}
	if deps[1].Type != ResourceTypeLagTrackerCommitTS || deps[1].ID != "n2_n3" {
		t.Errorf("expected dep on lag_tracker_commit_ts/n2_n3, got %v", deps[1])
	}
}

func TestReplicationSlotAdvanceEphemeral(t *testing.T) {
	r := NewReplicationSlotAdvanceFromCTS("n2", "n3", "app", nil, nil)
	if r.Status().Exists {
		t.Error("ephemeral resource should not exist")
	}
}

var _ resource.Resource = (*ReplicationSlotAdvanceFromCTS)(nil)

// --- ComputeDesired tests ---

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

	// sub from n1→n2 should have sync=true (populate sync subscription)
	subN1N2 := mustSubscription(t, resources, "sub_n1_n2")
	if !subN1N2.sync {
		t.Error("sub_n1_n2 should have sync=true")
	}

	// sub from n2→n1 should have sync=false
	subN2N1 := mustSubscription(t, resources, "sub_n2_n1")
	if subN2N1.sync {
		t.Error("sub_n2_n1 should have sync=false")
	}

	// Populate resources should be emitted (2-node add, no peers)
	assertResource(t, resources, ResourceTypeSyncEvent, "n1_n2")
	assertResource(t, resources, ResourceTypeWaitForSyncEvent, "n1_n2")
}

func TestComputeDesiredPopulateTwoNodeAdd(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "h1"},
			{Name: "n2", Hostname: "h2", Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"}},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil}
	resources := ComputeDesired(cfg, conns)

	// Should have populate resources: sync event, wait for sync event
	assertResource(t, resources, ResourceTypeSyncEvent, "n1_n2")
	assertResource(t, resources, ResourceTypeWaitForSyncEvent, "n1_n2")

	// Source→new subscription should have sync=true
	sub := mustSubscription(t, resources, "sub_n1_n2")
	if !sub.sync {
		t.Error("source→new subscription should have sync=true")
	}

	// Reverse sub n2→n1 should have extraDep on WaitForSyncEvent
	revSub := mustSubscription(t, resources, "sub_n2_n1")
	found := false
	for _, d := range revSub.Dependencies() {
		if d.Type == ResourceTypeWaitForSyncEvent && d.ID == "n1_n2" {
			found = true
		}
	}
	if !found {
		t.Error("sub_n2_n1 should depend on wait_for_sync_event/n1_n2")
	}

	// No ReplicationSlotCreate (no peers)
	for id := range resources {
		if id.Type == ResourceTypeReplicationSlotCreate {
			t.Errorf("unexpected ReplicationSlotCreate: %v", id)
		}
	}
}

func TestComputeDesiredPopulateThreeNodeAdd(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "h1"},
			{Name: "n2", Hostname: "h2"},
			{Name: "n3", Hostname: "h3", Bootstrap: config.NodeBootstrap{Mode: "spock", SourceNode: "n1"}},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil, "n3": nil}
	resources := ComputeDesired(cfg, conns)

	// Peer resources for n2
	assertResource(t, resources, ResourceTypeReplicationSlotCreate, "spk_app_n2_sub_n2_n3")
	assertResource(t, resources, ResourceTypeSyncEvent, "n2_n1")
	assertResource(t, resources, ResourceTypeWaitForSyncEvent, "n2_n1")
	assertResource(t, resources, ResourceTypeLagTrackerCommitTS, "n2_n3")
	assertResource(t, resources, ResourceTypeReplicationSlotAdvanceFromCTS, "n2_n3")

	// Source sync resources
	assertResource(t, resources, ResourceTypeSyncEvent, "n1_n3")
	assertResource(t, resources, ResourceTypeWaitForSyncEvent, "n1_n3")

	// Source→new subscription should have sync=true
	subN1N3 := mustSubscription(t, resources, "sub_n1_n3")
	if !subN1N3.sync {
		t.Error("sub_n1_n3 should have sync=true")
	}

	// Peer→new end-state subscription (sub_n2_n3) should depend on slot advance
	subN2N3 := mustSubscription(t, resources, "sub_n2_n3")
	foundAdvance := false
	for _, d := range subN2N3.Dependencies() {
		if d.Type == ResourceTypeReplicationSlotAdvanceFromCTS {
			foundAdvance = true
		}
	}
	if !foundAdvance {
		t.Error("sub_n2_n3 should depend on replication_slot_advance_from_cts")
	}

	// New→existing subs should depend on WaitForSyncEvent(source→new)
	for _, subID := range []string{"sub_n3_n1", "sub_n3_n2"} {
		s := mustSubscription(t, resources, subID)
		foundWait := false
		for _, d := range s.Dependencies() {
			if d.Type == ResourceTypeWaitForSyncEvent && d.ID == "n1_n3" {
				foundWait = true
			}
		}
		if !foundWait {
			t.Errorf("%s should depend on wait_for_sync_event/n1_n3", subID)
		}
	}
}

func TestComputeDesiredNoNewNodesUnchanged(t *testing.T) {
	cfg := &config.Config{
		DBName: "app", AdminUser: "admin", PgEdgeUser: "pgedge",
		Nodes: []config.Node{
			{Name: "n1", Hostname: "h1"},
			{Name: "n2", Hostname: "h2"},
		},
	}
	conns := map[string]*pgxpool.Pool{"n1": nil, "n2": nil}
	resources := ComputeDesired(cfg, conns)

	// 2 users + 2 nodes + 2 slots + 2 subscriptions = 8 (unchanged)
	if len(resources) != 8 {
		t.Fatalf("expected 8 resources, got %d", len(resources))
	}

	// No populate resources
	for id := range resources {
		switch id.Type {
		case ResourceTypeReplicationSlotCreate,
			ResourceTypeSyncEvent,
			ResourceTypeWaitForSyncEvent,
			ResourceTypeLagTrackerCommitTS,
			ResourceTypeReplicationSlotAdvanceFromCTS:
			t.Errorf("unexpected populate resource: %v", id)
		}
	}

	// Subscriptions should have no extraDeps
	for id, r := range resources {
		if id.Type != ResourceTypeSubscription {
			continue
		}
		s, ok := r.(*Subscription)
		if !ok {
			t.Fatalf("subscription %s has type %T, want *Subscription", id.ID, r)
		}
		if len(s.extraDeps) > 0 {
			t.Errorf("%s has unexpected extraDeps: %v", id.ID, s.extraDeps)
		}
	}
}

func assertResource(t *testing.T, resources map[resource.Identifier]resource.Resource, resType, id string) {
	t.Helper()
	key := resource.Identifier{Type: resType, ID: id}
	if _, ok := resources[key]; !ok {
		t.Errorf("missing %s/%s", resType, id)
	}
}

func mustSubscription(t *testing.T, resources map[resource.Identifier]resource.Resource, id string) *Subscription {
	t.Helper()
	r, ok := resources[resource.Identifier{Type: ResourceTypeSubscription, ID: id}]
	if !ok {
		t.Fatalf("missing subscription %s", id)
	}
	s, ok := r.(*Subscription)
	if !ok {
		t.Fatalf("subscription %s has type %T, want *Subscription", id, r)
	}
	return s
}
