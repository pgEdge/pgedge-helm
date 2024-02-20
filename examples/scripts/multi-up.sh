#!/usr/bin/env bash

set -o errexit
set -o pipefail

scripts_dir=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
configs_dir=${scripts_dir}/../configs

# Set cwd to examples dir
cd ${scripts_dir}/..

# Create clusters
kind create cluster --config ${configs_dir}/multi/iad/kind.yaml
kind create cluster --config ${configs_dir}/multi/sfo/kind.yaml

# Install Cilium on IAD
cilium install --context kind-multi-iad \
	--set cluster.name=iad \
	--set cluster.id=1 \
	--set encryption.nodeEncryption=true \
	--set encryption.type=wireguard \
    --set hubble.enabled=true \
    --set hubble.relay.enabled=true \
    --set hubble.ui.enabled=true
cilium status --context kind-multi-iad --wait

# Copy Cilium CA from IAD to SFO
kubectl --context kind-multi-iad \
	-n kube-system \
	get secret cilium-ca \
	-o yaml \
	| kubectl --context kind-multi-sfo create -f -

# Install Cilium on SFO
cilium install --context kind-multi-sfo \
	--set cluster.name=sfo \
	--set cluster.id=2 \
	--set encryption.nodeEncryption=true \
	--set encryption.type=wireguard \
    --set hubble.enabled=true \
    --set hubble.relay.enabled=true \
    --set hubble.ui.enabled=true
cilium status --context kind-multi-sfo --wait

# Enable cluster mesh
cilium clustermesh enable --context kind-multi-iad --service-type NodePort
cilium clustermesh status --context kind-multi-iad --wait
cilium clustermesh enable --context kind-multi-sfo --service-type NodePort
cilium clustermesh status --context kind-multi-sfo --wait

# Connect cluster mesh
cilium clustermesh connect --context kind-multi-iad \
	--destination-context kind-multi-sfo

# It's easiest to install Submariner from inside the kind network, so we make a
# new kubeconfig file with internal addresses. After it's deployed we'll be able
# to use a locally-installed subctl with our regular kubeconfig on the host.
kind export kubeconfig \
    --internal \
    --kubeconfig ./kubeconfig \
    --name multi-iad
kind export kubeconfig \
    --internal \
    --kubeconfig ./kubeconfig \
    --name multi-sfo

# Shortcut for subctl command
subctl_docker() {
    docker run --rm -it \
    --network kind \
    --volume $(pwd):/workspace \
    --workdir /workspace \
    --env KUBECONFIG=/workspace/kubeconfig \
    quay.io/submariner/subctl:release-0.16 subctl $@
}

# Deploy submariner broker
subctl_docker deploy-broker \
    --context kind-multi-iad \
    --globalnet=false

# Join clusters
subctl_docker join broker-info.subm \
    --clusterid iad \
    --context kind-multi-iad \
    --check-broker-certificate=false \
    --natt=false \
    --globalnet=false
subctl_docker join broker-info.subm \
    --clusterid sfo \
    --context kind-multi-sfo \
    --check-broker-certificate=false \
    --natt=false \
    --globalnet=false
