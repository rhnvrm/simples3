# SimpleS3 development commands. Local integration tests use SeaweedFS.
default:
    @just --list

test:
    @go test -v ./...

test-local:
    @bash scripts/test-local.sh

tidy:
    @go mod tidy

fmt:
    @go fmt ./...

vet:
    @go vet ./...

seaweedfs-up:
    @docker compose up -d seaweedfs

seaweedfs-down:
    @docker compose down

seaweedfs-logs:
    @docker compose logs -f seaweedfs

seaweedfs-clean:
    @docker compose down --volumes

seaweedfs-reset: seaweedfs-clean setup

setup:
    @bash scripts/seaweedfs-setup.sh

dev-env: setup
    @echo "SeaweedFS S3 API: http://127.0.0.1:9000"

clean: seaweedfs-clean

status:
    @docker compose ps
