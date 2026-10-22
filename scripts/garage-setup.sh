#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/garage-env.sh

garage() { docker compose exec -T garage /garage "$@"; }
docker compose up -d --wait --wait-timeout 90

# Initialize a fresh single-node layout; repeated setup preserves existing data.
if garage status | grep -q 'NO ROLE ASSIGNED'; then
    node_id=$(garage node id -q)
    garage layout assign -z local -c 1G "${node_id%%@*}"
    garage layout apply --version 1
fi

for attempt in {1..30}; do
    if garage health -q; then break; fi
    if [ "$attempt" -eq 30 ]; then
        docker compose logs garage
        echo 'Garage cluster did not become healthy' >&2
        exit 1
    fi
    sleep 1
done

if ! garage key info "$AWS_S3_ACCESS_KEY" >/dev/null 2>&1; then
    garage key import --yes -n simples3-test "$AWS_S3_ACCESS_KEY" "$AWS_S3_SECRET_KEY"
fi
garage key allow --create-bucket "$AWS_S3_ACCESS_KEY"
if ! garage bucket info "$AWS_S3_BUCKET" >/dev/null 2>&1; then
    garage bucket create "$AWS_S3_BUCKET"
fi
garage bucket allow --read --write --owner "$AWS_S3_BUCKET" --key "$AWS_S3_ACCESS_KEY"
# Verify S3 authentication and bucket access as well as node/RPC readiness.
aws --endpoint-url "$AWS_S3_ENDPOINT" s3api head-bucket --bucket "$AWS_S3_BUCKET"
