mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
test_dir := $(patsubst %/,%,$(dir $(mkfile_path)))
root_dir := $(abspath $(test_dir)/..)

# Configuration
KUBECONTEXT ?= kind-pgedge-test
NAMESPACE ?= default
CHART_REF ?= $(root_dir)
CHART_VERSION ?=
HELM_REPO ?=
INIT_SPOCK_IMAGE ?= pgedge-helm-utils:dev
TIMEOUT ?= 10m
KIND_CLUSTER_NAME ?= pgedge-test
KIND_CONFIG ?= $(test_dir)/kind.yaml

# Export for Go tests
export KUBECONTEXT
export NAMESPACE
export CHART_REF
export CHART_VERSION
export HELM_REPO
export INIT_SPOCK_IMAGE
export TIMEOUT

# ---- Tools ----

GOTESTSUM := $(shell which gotestsum 2>/dev/null)

.PHONY: install-gotestsum
install-gotestsum:
	go install gotest.tools/gotestsum@latest

# ---- Unit Tests ----

.PHONY: test-unit
test-unit:
	cd $(root_dir) && go test -tags unit -v ./test/unit/...

# ---- Kind Cluster Management ----

.PHONY: kind-up
kind-up:
	kind delete cluster --name $(KIND_CLUSTER_NAME) 2>/dev/null || true
	kind create cluster --config $(KIND_CONFIG) --name $(KIND_CLUSTER_NAME)
	kubectl apply --context kind-$(KIND_CLUSTER_NAME) --server-side -f \
		https://raw.githubusercontent.com/pgEdge/pgedge-cnpg-dist/refs/heads/main/manifests/cloudnative-pg/v1.29.0/cnpg-1.29.0.yaml
	kubectl apply --context kind-$(KIND_CLUSTER_NAME) -f \
		https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
	kubectl --context kind-$(KIND_CLUSTER_NAME) wait --for=condition=Available deployment \
		-n cnpg-system cnpg-controller-manager --timeout=300s
	kubectl --context kind-$(KIND_CLUSTER_NAME) wait --for=condition=Available deployment \
		-n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=300s

.PHONY: kind-down
kind-down:
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-load-image
kind-load-image:
	$(MAKE) -C $(root_dir) docker-build-dev
	kind load docker-image pgedge-helm-utils:dev --name $(KIND_CLUSTER_NAME)

# ---- Integration Tests ----

GOTESTSUM_CMD = cd $(root_dir) && gotestsum --format testname --

# GO_TIMEOUT is derived from TIMEOUT, adding 10m buffer for go test overhead
GO_TIMEOUT := $(shell echo "$(TIMEOUT)" | awk -F'm' '{print ($$1 + 10) "m"}' 2>/dev/null || echo "120m")

.PHONY: test-integration
test-integration:
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) ./test/integration/...

.PHONY: test-integration-kind
test-integration-kind: kind-up kind-load-image
	$(MAKE) test-integration KUBECONTEXT=kind-$(KIND_CLUSTER_NAME) INIT_SPOCK_IMAGE=pgedge-helm-utils:dev
	$(MAKE) kind-down

# ---- Selective Test Runs ----
# Usage: make test-run RUN=TestSlotLossRecovery
#        make test-run RUN="TestSlotLoss|TestNodesRemoveNode"

.PHONY: test-run
test-run: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "$(RUN)" ./test/integration/...

.PHONY: test-install
test-install: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "TestDistributedInstall|TestSingleNodeInstall" ./test/integration/...

.PHONY: test-nodes
test-nodes: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "TestNodes" ./test/integration/...

.PHONY: test-recovery
test-recovery: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "TestSlotLoss|TestOrphanSlot|TestResetSpock|TestIdempotent" ./test/integration/...

.PHONY: test-reset
test-reset: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "TestResetSpock" ./test/integration/...

.PHONY: test-failover
test-failover: kind-load-image
	$(GOTESTSUM_CMD) -tags integration -v -timeout $(GO_TIMEOUT) \
		-run "TestUnplannedFailover" ./test/integration/...

# ---- All ----

.PHONY: test-all
test-all: test-unit test-integration-kind
