PGEDGE_IMAGE_TAG=$(shell grep imageTag values.yaml | awk '{ print $$NF }')
PGEDGE_IMAGE?=pgedge/pgedge:$(PGEDGE_IMAGE_TAG)

##################
# Single cluster #
##################

.PHONY: single-up
single-up:
	kind create cluster --config examples/configs/single/kind.yaml

.PHONY: single-down
single-down:
	kind delete cluster --name single

.PHONY: single-install
single-install:
	helm install \
		--kube-context kind-single \
		--values examples/configs/single/values.yaml \
		--wait \
		pgedge .

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
		./examples/scripts/enable-replication.sh

.PHONY: single-load-image
single-load-image:
	kind load docker-image $(PGEDGE_IMAGE) -n single

.PHONY: ctx-single
ctx-single:
	kubectl config set current-context kind-single

#################
# Multi cluster #
#################

.PHONY: multi-up
multi-up:
	./examples/scripts/multi-up.sh

.PHONY: multi-down
multi-down:
	kind delete cluster --name multi-iad
	kind delete cluster --name multi-sfo
	rm -f ./broker-info.subm ./kubeconfig

.PHONY: multi-install
multi-install:
	./examples/scripts/multi-install.sh

.PHONY: multi-uninstall
multi-uninstall:
	helm uninstall --kube-context kind-multi-iad pgedge
	helm uninstall --kube-context kind-multi-sfo pgedge
	kubectl --context kind-multi-sfo delete secret pgedge-users
	kubectl config set current-context kind-multi-iad
	subctl unexport service pgedge-hl
	kubectl config set current-context kind-multi-sfo
	subctl unexport service pgedge-hl

.PHONY: multi-create-test-table
multi-create-test-table:
	kubectl --context kind-multi-iad exec -it pgedge-iad-0 -- psql -U app defaultdb \
		-c "SELECT spock.replicate_ddl('CREATE TABLE public.users (id uuid, name text, PRIMARY KEY (id))');"

.PHONY: multi-enable-replication
multi-enable-replication:
	CONTEXTS='kind-multi-iad kind-multi-sfo' \
		./examples/scripts/enable-replication.sh

.PHONY: multi-load-image
multi-load-image:
	kind load docker-image $(PGEDGE_IMAGE) -n multi-iad
	kind load docker-image $(PGEDGE_IMAGE) -n multi-sfo

.PHONY: clean
clean:
	rm -f broker-info.subm* kubeconfig

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

.PHONY: generate-readme
generate-readme:
	helm-docs
