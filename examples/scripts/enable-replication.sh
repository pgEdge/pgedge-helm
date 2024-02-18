#!/usr/bin/env bash

set -o errexit
set -o pipefail

if [[ -z ${CONTEXTS} ]]; then
    echo "CONTEXTS env var is required"
fi

for context in ${CONTEXTS}; do
    selector=$(kubectl --context ${context} get services pgedge -o json \
        | jq -r '.spec.selector|to_entries|.[]|.key + "=" + .value')

    for pod in $(kubectl --context ${context} get pods --selector "${selector}" -o json | jq -r '.items[].metadata.name'); do
        kubectl --context ${context} exec -it ${pod} -- psql -U app defaultdb \
            -c "SELECT spock.repset_add_all_tables('default', ARRAY['public']);"
    done
done
