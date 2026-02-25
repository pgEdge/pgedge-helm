mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
test_dir := $(patsubst %/,%,$(dir $(mkfile_path)))
root_dir := $(abspath $(test_dir)/..)

# Configuration
KUBECONTEXT ?= kind-pgedge-test
NAMESPACE ?= default
CHART_REF ?= $(root_dir)
CHART_VERSION ?=
HELM_REPO ?=
INIT_SPOCK_IMAGE ?=
TIMEOUT ?= 300s
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

# ---- Unit Tests ----

.PHONY: test-unit
test-unit:
	cd $(test_dir) && go test -tags unit -v ./unit/...

# ---- Kind Cluster Management ----

.PHONY: kind-up
kind-up:
	kind delete cluster --name $(KIND_CLUSTER_NAME) 2>/dev/null || true
	kind create cluster --config $(KIND_CONFIG) --name $(KIND_CLUSTER_NAME)
	kubectl apply --context kind-$(KIND_CLUSTER_NAME) --server-side -f \
		https://raw.githubusercontent.com/pgEdge/pgedge-cnpg-dist/refs/heads/main/manifests/cloudnative-pg/v1.28.1/cnpg-1.28.1.yaml
	kubectl apply --context kind-$(KIND_CLUSTER_NAME) -f \
		https://github.com/cert-manager/cert-manager/releases/download/v1.19.3/cert-manager.yaml
	kubectl --context kind-$(KIND_CLUSTER_NAME) wait --for=condition=Available deployment \
		-n cnpg-system cnpg-controller-manager --timeout=120s
	kubectl --context kind-$(KIND_CLUSTER_NAME) wait --for=condition=Available deployment \
		-n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=120s

.PHONY: kind-down
kind-down:
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-load-image
kind-load-image:
	$(MAKE) -C $(root_dir) docker-build-dev
	kind load docker-image pgedge-helm-utils:dev --name $(KIND_CLUSTER_NAME)

# ---- Integration Tests ----

.PHONY: test-integration
test-integration:
	cd $(test_dir) && go test -tags integration -v -timeout 20m ./integration/...

.PHONY: test-integration-kind
test-integration-kind: kind-up kind-load-image
	$(MAKE) test-integration KUBECONTEXT=kind-$(KIND_CLUSTER_NAME) INIT_SPOCK_IMAGE=pgedge-helm-utils:dev
	$(MAKE) kind-down

# ---- Selective Test Runs ----

.PHONY: test-install
test-install:
	cd $(test_dir) && go test -tags integration -v -timeout 20m -run "TestDistributedInstall|TestSingleNodeInstall" ./integration/...

.PHONY: test-nodes
test-nodes:
	cd $(test_dir) && go test -tags integration -v -timeout 20m -run "TestNodes" ./integration/...

# ---- All ----

.PHONY: test-all
test-all: test-unit test-integration-kind
