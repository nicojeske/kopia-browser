.PHONY: run test test-integration build docker

# Run locally. godotenv loads .env from the working directory automatically.
run:
	go run ./cmd/kopia-browser

# Unit tests.
test:
	go test ./...

# Integration tests against real garage (build tag: integration). Tests skip
# themselves when creds are absent. No integration tests exist yet (added M1).
test-integration:
	go test -tags=integration ./...

# Build the binary.
build:
	go build -o bin/kopia-browser ./cmd/kopia-browser

# Build the container image (Dockerfile arrives in M6).
docker:
	docker build -t kopia-browser .
