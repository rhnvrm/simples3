# SimpleS3 Development Commands

# Variables
MINIO_CONTAINER_NAME := "simples3-minio"
MINIO_DATA_DIR := ".minio-data"
AWS_S3_BUCKET := "testbucket"

# Default command - lists all available recipes
default:
    @just --list

# --- Development Commands ---
test:
    @echo "🧪 Running all tests..."
    @go test -v ./...

test-local: setup
    @echo "🧪 Running tests with local MinIO..."
    @sleep 2
    @go test -v ./...


# --- Go Module Management ---
tidy:
    @echo "📦 Tidying Go modules..."
    @go mod tidy

fmt:
    @echo "🎨 Formatting Go code..."
    @go fmt ./...

vet:
    @echo "🔍 Running go vet..."
    @go vet ./...

# --- MinIO Management ---
minio-up:
    @echo "🚀 Starting MinIO container..."
    @docker compose up -d
    @echo "✅ MinIO started:"
    @echo "   API: http://localhost:9000"
    @echo "   Console: http://localhost:9001"
    @echo "   Access Key: minioadmin"
    @echo "   Secret Key: minioadmin"

minio-down:
    @echo "🛑 Stopping MinIO container..."
    @docker compose down

minio-logs:
    @echo "📋 Showing MinIO logs..."
    @docker compose logs -f

minio-clean:
    @echo "🧹 Cleaning MinIO data..."
    @docker compose down --volumes

minio-reset: minio-clean minio-up

# --- Development Environment Setup ---
setup: minio-up
    @echo "⚙️ Setting up development environment..."
    @sleep 3
    @aws --endpoint-url http://127.0.0.1:9000/ s3 mb s3://{{AWS_S3_BUCKET}} || true
    @echo "✅ Development environment ready!"

dev-env: setup
    @echo "🎯 Development environment active!"
    @echo "   MinIO: http://localhost:9000"
    @echo "   Console: http://localhost:9001"
    @echo "   Bucket: {{AWS_S3_BUCKET}}"

# --- Cleanup Commands ---
clean:
    @echo "🧹 Cleaning up development environment..."
    @docker compose down --volumes

# --- Helper Commands ---
status:
    @echo "📊 Status Check:"
    @echo "   MinIO Container: $(docker ps -q -f name={{MINIO_CONTAINER_NAME}} | wc -l | tr -d ' ') running"
    @if [ "$(docker ps -q -f name={{MINIO_CONTAINER_NAME}})" ]; then \
        echo "   MinIO URL: http://localhost:9000"; \
        echo "   Console URL: http://localhost:9001"; \
    fi
    @echo "   Go version: $(go version | awk '{print $3}')"

# --- Documentation ---
docs:
    @echo "📖 Opening documentation..."
    @echo "   Plan: PLAN.md"
    @echo "   README: README.md"