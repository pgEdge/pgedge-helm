.PHONY: gen-docs
gen-docs:
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs
	docker run --rm -v ${PWD}:/helm-docs jnorwood/helm-docs -t docs/configuration.md.gotmpl -o docs/configuration.md

.PHONY: docs
docs:
	docker build -t pgedge-helm-docs ./docs
	docker run --rm -it -p 8000:8000 -v ${PWD}:/docs pgedge-helm-docs
