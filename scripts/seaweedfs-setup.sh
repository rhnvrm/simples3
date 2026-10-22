#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/seaweedfs-env.sh

docker compose up -d seaweedfs

# Verify authenticated S3 access, not just process/port readiness. mini seeds the
# test identity and bucket on first startup and preserves them on later starts.
for attempt in {1..60}; do
    if AWS_MAX_ATTEMPTS=1 aws --endpoint-url "$AWS_S3_ENDPOINT" --cli-connect-timeout 2 --cli-read-timeout 2 \
        s3api head-bucket --bucket "$AWS_S3_BUCKET" >/dev/null 2>&1; then
        echo "SeaweedFS is ready at $AWS_S3_ENDPOINT"
        exit 0
    fi
    if [ "$attempt" -eq 60 ]; then
        docker compose logs seaweedfs
        echo 'SeaweedFS S3 endpoint did not become ready' >&2
        exit 1
    fi
    sleep 1
done
