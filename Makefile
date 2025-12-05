# Get chart version from Chart.yaml
CHART_VERSION := $(shell grep '^version:' Chart.yaml | awk '{print $$2}')
IMAGE_NAME := pgedge-helm-utils
IMAGE_TAG := $(CHART_VERSION)
REGISTRY ?= ghcr.io/pgedge

.PHONY: gen-docs
gen-docs:
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs -t docs/configuration.md.gotmpl -o docs/configuration.md

.PHONY: docs
docs:
	docker build -t pgedge-helm-docs ./docs
	docker run --rm -it -p 8000:8000 -v ${PWD}:/docs pgedge-helm-docs

.PHONY: docker-build
docker-build:
	docker buildx bake --set release.tags=$(IMAGE_NAME):$(IMAGE_TAG) --set release.tags=$(IMAGE_NAME):latest release
	@echo "Built $(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: docker-build-dev
docker-build-dev:
	docker buildx bake dev
	@echo "Built $(IMAGE_NAME):dev for local development"

.PHONY: docker-push
docker-push:
	CHART_VERSION=$(CHART_VERSION) REGISTRY=$(REGISTRY) docker buildx bake push
	@echo "Pushed $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"

.PHONY: docker-load-kind
docker-load-kind: docker-build-dev
	kind load docker-image $(IMAGE_NAME):dev --name single
	@echo "Loaded $(IMAGE_NAME):dev into kind"
