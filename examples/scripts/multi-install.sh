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

# Export pgedge headless service from IAD
# This creates the global DNS entries for each node.
kubectl config set current-context kind-multi-iad
subctl export service pgedge-hl

# Copy users secret from IAD to SFO
kubectl --context kind-multi-iad \
	get secret pgedge-users \
	-o yaml \
	| kubectl --context kind-multi-sfo create -f -

# Deploy pgedge to SFO
echo "Deploying pgedge to SFO"
helm install \
    --kube-context kind-multi-sfo \
    --values ${configs_dir}/multi/sfo/values.yaml \
    --wait \
    pgedge .

# Export pgedge headless service from SFO
kubectl config set current-context kind-multi-sfo
subctl export service pgedge-hl
