README.md: README.md.gotmpl values.yaml
	helm-docs

docs/configuration.md: docs/configuration.md.gotmpl values.yaml
	helm-docs -t docs/configuration.md.gotmpl -o docs/configuration.md

.PHONY: gen-docs
gen-docs: README.md docs/configuration.md

.PHONY: docs
docs:
	docker run --rm -it -p 8000:8000 -v ${PWD}:/docs squidfunk/mkdocs-material
