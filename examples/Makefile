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

##################
# Multi cluster #
##################

.PHONY: multi-up
multi-up:
	kind create cluster --config $(examples_dir)/configs/multi/cluster-a.yaml
	kind create cluster --config $(examples_dir)/configs/multi/cluster-b.yaml
	CLUSTER=cluster-a CLUSTER_ID=1 make install-prereqs
	CLUSTER=cluster-b CLUSTER_ID=2 make install-prereqs
	cilium clustermesh connect \
		--context kind-cluster-a \
		--destination-context kind-cluster-b

.PHONY: install-prereqs
install-prereqs:
	kubectl apply --context kind-$(CLUSTER) --server-side -f \
		https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.27/releases/cnpg-1.27.1.yaml
	kubectl apply --context kind-$(CLUSTER) -f \
		https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
	kubectl apply --context kind-$(CLUSTER) -f \
		https://raw.githubusercontent.com/kubernetes-sigs/mcs-api/62ede9a032dcfbc41b3418d7360678cb83092498/config/crd/multicluster.x-k8s.io_serviceexports.yaml
	kubectl apply --context kind-$(CLUSTER) -f \
		https://raw.githubusercontent.com/kubernetes-sigs/mcs-api/62ede9a032dcfbc41b3418d7360678cb83092498/config/crd/multicluster.x-k8s.io_serviceimports.yaml
	helm repo add cilium https://helm.cilium.io --force-update
	helm repo update
	helm install cilium cilium/cilium \
		--version v1.19.0-pre.2 \
		--namespace kube-system \
		--create-namespace \
		--kube-context kind-$(CLUSTER) \
		--set cluster.name=$(CLUSTER) \
		--set cluster.id=$(CLUSTER_ID) \
		--set kubeProxyReplacement=false \
		--set clustermesh.enabled=true \
		--set clustermesh.mcsapi.enabled=true \
		--set clustermesh.mcsapi.corednsAutoConfigure.enabled=true
	cilium clustermesh enable \
		--context kind-$(CLUSTER) \
		--service-type NodePort \
		--enable-kvstoremesh=false
	
.PHONY: multi-down
multi-down:
	kind delete cluster --name cluster-a
	kind delete cluster --name cluster-b

.PHONY: multi-install
multi-install:
	helm install \
		--kube-context kind-cluster-a \
		--values $(examples_dir)/configs/multi/values-a.yaml \
		--wait \
		pgedge $(root_dir)
	kubectl --context kind-cluster-a wait --for=condition=Ready cluster/pgedge-n1 -n default --timeout=120s
	CLUSTER_SRC=cluster-a CLUSTER_DST=cluster-b make copy-certs
	helm install \
		--kube-context kind-cluster-b \
		--values $(examples_dir)/configs/multi/values-b.yaml \
		--wait \
		pgedge $(root_dir)
	kubectl apply --context kind-cluster-a -f \
		$(examples_dir)/configs/multi/cilium-a.yaml
	kubectl apply --context kind-cluster-b -f \
		$(examples_dir)/configs/multi/cilium-b.yaml

.PHONY: multi-upgrade
multi-upgrade:
	helm upgrade \
		--kube-context kind-cluster-a \
		--values $(examples_dir)/configs/multi/values-a.yaml \
		--wait \
		pgedge $(root_dir)
	helm upgrade \
		--kube-context kind-cluster-b \
		--values $(examples_dir)/configs/multi/values-b.yaml \
		--wait \
		pgedge $(root_dir)
		
.PHONY: copy-certs
copy-certs:
	@for secret in admin-client-cert app-client-cert client-ca-key-pair pgedge-client-cert streaming-replica-client-cert; do \
		if ! kubectl get secret $$secret -n default --context kind-$(CLUSTER_DST) >/dev/null 2>&1; then \
			kubectl get secret $$secret -n default --context kind-$(CLUSTER_SRC) -o yaml | \
			kubectl apply --context kind-$(CLUSTER_DST) -f -; \
		else \
			echo "Secret $$secret already exists in kind-$(CLUSTER_DST), skipping."; \
		fi \
	done

.PHONY: multi-uninstall
multi-uninstall:
	helm uninstall --kube-context kind-cluster-a pgedge
	helm uninstall --kube-context kind-cluster-b pgedge