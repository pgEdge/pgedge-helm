mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
examples_dir := $(patsubst %/,%,$(dir $(mkfile_path)))
root_dir := $(abspath $(examples_dir)/..)

##################
# Single cluster #
##################

.PHONY: single-up
single-up:
	kind create cluster --config $(examples_dir)/configs/single/kind.yaml
	kubectl apply --context kind-single --server-side -f \
		https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.27/releases/cnpg-1.27.1.yaml
	kubectl apply --context kind-single -f \
		https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
	kubectl --context kind-single wait --for=condition=Available deployment \
		-n cert-manager cert-manager cert-manager-cainjector cert-manager-webhook --timeout=120s
		
.PHONY: single-down
single-down:
	kind delete cluster --name single

.PHONY: single-install
single-install:
	helm install \
		--kube-context kind-single \
		--values $(examples_dir)/configs/single/values.yaml \
		--wait \
		pgedge $(root_dir)

.PHONY: single-uninstall
single-uninstall:
	helm uninstall --kube-context kind-single pgedge

.PHONY: single-test
single-test:
	helm test --kube-context kind-single pgedge
	
.PHONY: single-upgrade
single-upgrade:
	helm upgrade \
		--kube-context kind-single \
		--values $(examples_dir)/configs/single/values.yaml \
		--wait \
		pgedge $(root_dir)

.PHONY: ctx-single
ctx-single:
	kubectl config set current-context kind-single

.PHONY: clean
clean:
	rm -f $(examples_dir)/broker-info.subm* $(examples_dir)/kubeconfig