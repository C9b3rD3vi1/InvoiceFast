.PHONY: all build test vet lint run clean cover bench dev

APP_NAME   := invoicefast
BUILD_DIR  := ./build
CMD_DIR    := ./cmd/server
MAIN_FILE  := $(CMD_DIR)/main.go

all: lint test build

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

test:
	@echo "Running tests..."
	go test ./... -count=1 -timeout 120s

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running linters..."
	go vet ./...
	@echo "Vet passed. Install golangci-lint for deeper analysis: https://golangci-lint.run/"

run:
	@echo "Starting $(APP_NAME)..."
	go run $(MAIN_FILE)

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	go clean -cache

cover:
	@echo "Running tests with coverage..."
	mkdir -p $(BUILD_DIR)
	go test ./... -count=1 -timeout 120s -coverprofile=$(BUILD_DIR)/coverage.out
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"
	go tool cover -func=$(BUILD_DIR)/coverage.out

bench:
	@echo "Running benchmarks..."
	go test ./... -bench=. -benchmem -count=1 -timeout 300s

dev:
	@echo "Starting $(APP_NAME) in development mode..."
	APP_ENV=development go run $(MAIN_FILE)

tidy:
	@echo "Tidying modules..."
	go mod tidy
	go mod verify

fmt:
	@echo "Formatting code..."
	go fmt ./...

.PHONY: security
security:
	@echo "Running security checks..."
	go vet ./...
	@echo "For deeper audit, install and run:"
	@echo "  go install github.com/securego/gosec/v2/cmd/gosec@latest && gosec ./..."
