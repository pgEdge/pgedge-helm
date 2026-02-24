# Get chart version from Chart.yaml
CHART_VERSION := $(shell grep '^version:' Chart.yaml | awk '{print $$2}')
BUILD_REVISION ?= 1
IMAGE_NAME := pgedge-helm-utils
IMAGE_TAG := $(CHART_VERSION)-$(BUILD_REVISION)
REGISTRY ?= ghcr.io/pgedge
BUILDX_BUILDER ?= pgedge-helm-builder 

# ---- Setup ----

.PHONY: setup
setup:
	brew install helm chart-testing yamllint kind go changie

.PHONY: buildx-init
buildx-init:
	docker buildx use $(BUILDX_BUILDER) || docker buildx create --name $(BUILDX_BUILDER) --platform=linux/arm64,linux/amd64

.PHONY: gen-docs
gen-docs:
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs -t docs/configuration.md.gotmpl -o docs/configuration.md

.PHONY: docs
docs:
	docker build -t pgedge-helm-docs ./docs
	docker run --rm -it -p 8000:8000 -v ${PWD}:/docs pgedge-helm-docs

.PHONY: docker-build-dev
docker-build-dev:
	docker buildx bake dev

.PHONY: docker-push-dev
docker-push-dev: buildx-init
	REGISTRY=$(REGISTRY) docker buildx bake dev-push --builder $(BUILDX_BUILDER)

.PHONY: docker-release
docker-release: buildx-init
	CHART_VERSION=$(CHART_VERSION) BUILD_REVISION=$(BUILD_REVISION) REGISTRY=$(REGISTRY) docker buildx bake release --builder $(BUILDX_BUILDER)

# Release targets
changie := $(shell command -v changie 2> /dev/null)

.PHONY: release
release:
ifndef changie
	$(error changie is not installed. Install it from https://changie.dev/guide/installation/)
endif
ifndef VERSION
	$(error VERSION must be set to trigger a release)
endif
	@echo "Creating release $(VERSION)..."
	$(changie) batch $(VERSION)
	$(changie) merge
	@# Update Chart.yaml version
	sed -i.bak 's/^version: .*/version: $(subst v,,$(VERSION))/' Chart.yaml && rm Chart.yaml.bak
	@# Update docs with hardcoded versions (handled by gen-docs for README)
	$(MAKE) gen-docs
	@# Copy CHANGELOG to docs for mkdocs
	cp CHANGELOG.md docs/changelog.md
	git checkout -b release/$(VERSION)
	git add -A
	git commit -m "build(release): bump version to $(VERSION)"
	git push origin release/$(VERSION)
	git tag -a -F changes/$(VERSION).md $(VERSION)-rc.1
	git push origin $(VERSION)-rc.1
	@echo ""
	@echo "Release branch and RC tag created successfully!"
	@echo "Open PR: https://github.com/pgedge/pgedge-helm/compare/release/$(VERSION)?expand=1"

.PHONY: major-release
major-release:
ifndef changie
	$(error changie is not installed. Install it from https://changie.dev/guide/installation/)
endif
	$(MAKE) release VERSION=$(shell $(changie) next major)

.PHONY: minor-release
minor-release:
ifndef changie
	$(error changie is not installed. Install it from https://changie.dev/guide/installation/)
endif
	$(MAKE) release VERSION=$(shell $(changie) next minor)

.PHONY: patch-release
patch-release:
ifndef changie
	$(error changie is not installed. Install it from https://changie.dev/guide/installation/)
endif
	$(MAKE) release VERSION=$(shell $(changie) next patch)

.PHONY: print-next-versions
print-next-versions:
ifndef changie
	$(error changie is not installed. Install it from https://changie.dev/guide/installation/)
endif
	@echo "Next major version: $(shell $(changie) next major)"
	@echo "Next minor version: $(shell $(changie) next minor)"
	@echo "Next patch version: $(shell $(changie) next patch)"

.PHONY: ct-lint
ct-lint:
	ct lint --chart-dirs . --charts .

.PHONY: ct-install
ct-install:
	ct install --chart-dirs . --charts . --upgrade --skip-missing-values --github-groups --helm-extra-set-args "--set pgEdge.initSpockImageName=pgedge-helm-utils:dev"

.PHONY: test-unit
test-unit:
	$(MAKE) -C test test-unit

.PHONY: test-integration
test-integration:
	$(MAKE) -C test test-integration

.PHONY: test-integration-kind
test-integration-kind:
	$(MAKE) -C test test-integration-kind

.PHONY: test-all
test-all:
	$(MAKE) -C test test-all
