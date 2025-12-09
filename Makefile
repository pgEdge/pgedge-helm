# Get chart version from Chart.yaml
CHART_VERSION := $(shell grep '^version:' Chart.yaml | awk '{print $$2}')
BUILD_REVISION ?= 1
IMAGE_NAME := pgedge-helm-utils
IMAGE_TAG := $(CHART_VERSION)-$(BUILD_REVISION)
REGISTRY ?= ghcr.io/pgedge

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

.PHONY: docker-release
docker-release:
	CHART_VERSION=$(CHART_VERSION) BUILD_REVISION=$(BUILD_REVISION) REGISTRY=$(REGISTRY) docker buildx bake release

