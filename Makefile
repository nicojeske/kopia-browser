.PHONY: run test test-integration e2e screenshots build docker

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

# Browser end-to-end tests (build tag: e2e). Drives headless Chrome against the
# server backed by a fake data layer; needs Chrome/Chromium installed.
e2e:
	go test -tags=e2e ./internal/web/...

# Capture UI screenshots using headless Chrome against the fake data layer.
# Output: docs/screenshots/*.png — commit these for the README.
# Requires Chrome/Chromium installed.
screenshots:
	go test -tags=screenshots -run TestCaptureScreenshots ./internal/web/...

# Build the binary.
build:
	go build -o bin/kopia-browser ./cmd/kopia-browser

# Build the container image (Dockerfile arrives in M6).
docker:
	docker build -t kopia-browser .
