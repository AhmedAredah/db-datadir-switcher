# DBSwitcher Makefile
# Author: Ahmed Aredah

.PHONY: all build build-all clean test install deps help

# Build configuration
BINARY_NAME=dbswitcher
VERSION?=$(shell git describe --tags --exact-match 2>/dev/null || echo "dev")
BUILD_DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildDate=$(BUILD_DATE)"

# Platform-specific binary extension
ifeq ($(OS),Windows_NT)
    BINARY_EXT=.exe
else
    BINARY_EXT=
endif

# Directories
BUILD_DIR=build
DIST_DIR=dist

# Default target
all: clean test build

## build: Build binary for current platform
build:
	@echo "Building $(BINARY_NAME) for current platform..."
	go build $(LDFLAGS) -o $(BINARY_NAME)$(BINARY_EXT) main.go
	@echo "✓ Build complete: $(BINARY_NAME)$(BINARY_EXT)"

## build-all: Build binaries for all platforms
build-all: clean
	@echo "Building $(BINARY_NAME) for all platforms..."
	@mkdir -p $(DIST_DIR)
	
	# Windows AMD64
	@echo "Building for Windows AMD64..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe main.go
	
	# Linux AMD64
	@echo "Building for Linux AMD64..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 main.go
	
	# Linux ARM64
	@echo "Building for Linux ARM64..."
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 main.go
	
	# macOS AMD64
	@echo "Building for macOS AMD64..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 main.go
	
	# macOS ARM64
	@echo "Building for macOS ARM64..."
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 main.go
	
	# FreeBSD AMD64
	@echo "Building for FreeBSD AMD64..."
	GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-freebsd-amd64 main.go
	
	@echo "✓ All builds complete. Binaries in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

## build-release: Build optimized release binaries
build-release: 
	@$(MAKE) build-all LDFLAGS="-ldflags '-s -w -X main.Version=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -extldflags=-static'"

## test: Run tests
test:
	@echo "Running tests..."
	go test -v ./...
	@echo "✓ Tests complete"

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report generated: coverage.html"

## lint: Run linters
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
		gofmt -l .; \
	fi
	@echo "✓ Linting complete"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "✓ Dependencies updated"

## deps-update: Update dependencies
deps-update:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✓ Dependencies updated"

## install: Install binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) main.go
	@echo "✓ $(BINARY_NAME) installed"

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME) $(BINARY_NAME).exe $(BINARY_NAME)$(BINARY_EXT)
	@rm -rf $(BUILD_DIR) $(DIST_DIR)
	@rm -f coverage.out coverage.html
	@echo "✓ Clean complete"

## run: Build and run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)$(BINARY_EXT)

## run-gui: Run in GUI mode
run-gui: build
	@echo "Running $(BINARY_NAME) in GUI mode..."
	./$(BINARY_NAME)$(BINARY_EXT) gui

## run-tray: Run in system tray mode
run-tray: build
	@echo "Running $(BINARY_NAME) in tray mode..."
	./$(BINARY_NAME)$(BINARY_EXT) tray

## run-cli: Run CLI help
run-cli: build
	@echo "Running $(BINARY_NAME) CLI..."
	./$(BINARY_NAME)$(BINARY_EXT) help

## dev: Run in development mode with race detection
dev:
	@echo "Running in development mode..."
	go run -race main.go

## format: Format source code
format:
	@echo "Formatting source code..."
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi
	@echo "✓ Formatting complete"

## checksums: Generate checksums for dist files
checksums:
	@echo "Generating checksums..."
	@if [ -d "$(DIST_DIR)" ]; then \
		cd $(DIST_DIR) && sha256sum * > checksums.sha256; \
		echo "✓ Checksums generated: $(DIST_DIR)/checksums.sha256"; \
	else \
		echo "⚠ No dist directory found. Run 'make build-all' first."; \
	fi

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t dbswitcher:$(VERSION) .
	@echo "✓ Docker image built: dbswitcher:$(VERSION)"

## docker-run: Run Docker container
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm -it dbswitcher:$(VERSION)

## release-prep: Prepare for release (format, test, build-all, checksums)
release-prep: format test build-all checksums
	@echo "✓ Release preparation complete"
	@echo "Version: $(VERSION)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Files ready in $(DIST_DIR)/"

## size: Show binary sizes
size:
	@if [ -d "$(DIST_DIR)" ]; then \
		echo "Binary sizes:"; \
		ls -lah $(DIST_DIR)/ | grep -E "(\.exe|dbswitcher-)" | awk '{printf "%-30s %8s\n", $$9, $$5}'; \
	else \
		echo "⚠ No binaries found. Run 'make build-all' first."; \
	fi

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## profile: Generate CPU profile
profile: build
	@echo "Generating CPU profile..."
	./$(BINARY_NAME) -cpuprofile=cpu.prof status
	go tool pprof cpu.prof

## mod-graph: Show module dependency graph
mod-graph:
	@echo "Module dependency graph:"
	go mod graph | head -20

## help: Show this help message
help:
	@echo "DBSwitcher - MariaDB Configuration Manager"
	@echo "Author: Ahmed Aredah"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Environment variables:"
	@echo "  VERSION     - Set version (default: git describe or 'dev')"
	@echo "  BUILD_DATE  - Set build date (default: current UTC time)"
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make build-all          # Build for all platforms"
	@echo "  make test               # Run tests"
	@echo "  make release-prep       # Prepare release package"
	@echo "  make VERSION=0.0.1 build-all  # Build with specific version"