mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
examples_dir := $(patsubst %/,%,$(dir $(mkfile_path)))
root_dir := $(abspath $(examples_dir)/..)

pgedge_image_tag ?= $(shell grep imageTag $(root_dir)/values.yaml | awk '{ print $$NF }')
pgedge_image ?= pgedge/pgedge:$(pgedge_image_tag)

##################
# Single cluster #
##################

.PHONY: single-up
single-up:
	kind create cluster --config $(examples_dir)/configs/single/kind.yaml

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

.PHONY: single-create-test-table
single-create-test-table:
	kubectl --context kind-single exec -it pgedge-0 -- psql -U app defaultdb \
		-c "SELECT spock.replicate_ddl('CREATE TABLE public.users (id uuid, name text, PRIMARY KEY (id))');"

.PHONY: single-enable-replication
single-enable-replication:
	CONTEXTS='kind-single' \
		$(examples_dir)/scripts/enable-replication.sh

.PHONY: single-load-image
single-load-image:
	kind load docker-image $(pgedge_image) -n single

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

#################
# Multi cluster #
#################

.PHONY: multi-up
multi-up:
	$(examples_dir)/scripts/multi-up.sh

.PHONY: multi-down
multi-down:
	kind delete cluster --name multi-iad
	kind delete cluster --name multi-sfo
	rm -f $(examples_dir)/broker-info.subm $(examples_dir)/kubeconfig

.PHONY: multi-install
multi-install:
	$(examples_dir)/scripts/multi-install.sh

.PHONY: multi-uninstall
multi-uninstall:
	helm uninstall --kube-context kind-multi-iad --ignore-not-found pgedge 
	helm uninstall --kube-context kind-multi-sfo --ignore-not-found pgedge 
	helm uninstall --kube-context kind-multi-sfo --ignore-not-found pgedge-init
	kubectl --context kind-multi-sfo delete secret --ignore-not-found=true client-ca-key-pair
	kubectl --context kind-multi-sfo delete secret --ignore-not-found=true pgedge-client-cert
	kubectl --context kind-multi-sfo delete secret --ignore-not-found=true admin-client-cert
	kubectl --context kind-multi-sfo delete secret --ignore-not-found=true streaming-replica-client-cert
	kubectl config set current-context kind-multi-iad
	-subctl unexport service pgedge-iad-n1-rw
	-subctl unexport service pgedge-iad-n2-rw
	kubectl config set current-context kind-multi-sfo
	-subctl unexport service pgedge-sfo-n3-rw
	-subctl unexport service pgedge-sfo-n4-rw

.PHONY: multi-create-test-table
multi-create-test-table:
	kubectl --context kind-multi-iad exec -it pgedge-iad-0 -- psql -U app defaultdb \
		-c "SELECT spock.replicate_ddl('CREATE TABLE public.users (id uuid, name text, PRIMARY KEY (id))');"

.PHONY: multi-enable-replication
multi-enable-replication:
	CONTEXTS='kind-multi-iad kind-multi-sfo' \
		$(examples_dir)/scripts/enable-replication.sh

.PHONY: multi-load-image
multi-load-image:
	kind load docker-image $(pgedge_image) -n multi-iad
	kind load docker-image $(pgedge_image) -n multi-sfo

.PHONY: multi-upgrade
multi-upgrade:
	helm upgrade \
		--kube-context kind-multi-iad \
		--values $(examples_dir)/configs/multi/iad/values.yaml \
		--wait \
		pgedge $(root_dir)
	helm upgrade \
		--kube-context kind-multi-sfo \
		--values $(examples_dir)/configs/multi/sfo/values.yaml \
		--wait \
		pgedge $(root_dir)

.PHONY: multi-test
multi-test:
	helm test --kube-context kind-multi-iad pgedge
	helm test --kube-context kind-multi-sfo pgedge

.PHONY: clean
clean:
	rm -f $(examples_dir)/broker-info.subm* $(examples_dir)/kubeconfig

.PHONY: ctx-iad
ctx-iad:
	kubectl config set current-context kind-multi-iad

.PHONY: ctx-sfo
ctx-sfo:
	kubectl config set current-context kind-multi-sfo

##########
# Common #
##########

.PHONY: run-client
run-client:
	kubectl run -it --rm client \
		--image=postgres:16.2 \
		--restart=Never -- bash

.PHONY: print-secret
print-secret:
	kubectl get secrets pgedge-users -o json \
		| jq '.data["users.json"]|@base64d|fromjson'
