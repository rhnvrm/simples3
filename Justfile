# SimpleS3 development commands. Local integration tests use Garage.
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

garage-up:
    @docker compose up -d --wait --wait-timeout 90

garage-down:
    @docker compose down

garage-logs:
    @docker compose logs -f garage

garage-clean:
    @docker compose down --volumes

garage-reset: garage-clean setup

setup:
    @bash scripts/garage-setup.sh

dev-env: setup
    @echo "Garage S3 API: http://127.0.0.1:9000"

clean: garage-clean

status:
    @docker compose ps
    @docker compose exec -T garage /garage status
