#!/usr/bin/env bash

set -o errexit
set -o pipefail

scripts_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
configs_dir=${scripts_dir}/../configs

# Set cwd to repo root dir
cd ${scripts_dir}/../..

# Deploy pgedge to IAD
echo "Deploying pgedge to IAD"
helm install \
    --kube-context kind-multi-iad \
    --values ${configs_dir}/multi/iad/values.yaml \
    --wait \
    pgedge .

kubectl wait --context kind-multi-iad \
    --for=condition=Ready cluster/pgedge-iad-n1
kubectl wait --context kind-multi-iad \
    --for=condition=Ready cluster/pgedge-iad-n2

# Export pgedge headless service from IAD
# This creates the global DNS entries for each node.
kubectl config set current-context kind-multi-iad
subctl export service pgedge-iad-n1-rw
subctl export service pgedge-iad-n2-rw

# Copy pgedge-client-cert and admin-client-cert secrets from IAD to SFO
kubectl --context kind-multi-iad \
	get secret client-ca-key-pair \
	-o yaml \
	| kubectl --context kind-multi-sfo apply -f -

kubectl --context kind-multi-iad \
	get secret pgedge-client-cert \
	-o yaml \
	| kubectl --context kind-multi-sfo apply -f -

kubectl --context kind-multi-iad \
	get secret admin-client-cert \
	-o yaml \
	| kubectl --context kind-multi-sfo apply -f -

kubectl --context kind-multi-iad \
    get secret streaming-replica-client-cert \
    -o yaml \
    | kubectl --context kind-multi-sfo apply -f -

# Deploy pgedge to SFO
echo "Deploying pgedge to SFO"
helm install \
    --kube-context kind-multi-sfo \
    --values ${configs_dir}/multi/sfo/values.yaml \
    --wait \
    pgedge .

kubectl wait --context kind-multi-sfo \
    --for=condition=Ready cluster/pgedge-sfo-n3
kubectl wait --context kind-multi-sfo \
    --for=condition=Ready cluster/pgedge-sfo-n4

# Export pgedge headless service from SFO
kubectl config set current-context kind-multi-sfo
subctl export service pgedge-sfo-n3-rw
subctl export service pgedge-sfo-n4-rw

# Init spock across both clusters in sfo
echo "Deploying pgedge-init to SFO"
helm install \
    --kube-context kind-multi-sfo \
    --values ${configs_dir}/multi/sfo/init.yaml \
    --wait \
    pgedge-init .