.PHONY: test build vet fmt check ci clean

# Run all tests with race detection and coverage
test:
	@echo "==> Running tests with race detection..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic .
	@echo "\n==> Test coverage summary:"
	@go tool cover -func=coverage.out

# Build all packages
build:
	@echo "==> Building all packages..."
	@go build -v ./...

# Run go vet
vet:
	@echo "==> Running go vet..."
	@go vet ./...

# Check code formatting
fmt:
	@echo "==> Checking code formatting..."
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "ERROR: The following files are not formatted:"; \
		gofmt -l .; \
		echo "\nRun 'make fmt-fix' to fix formatting issues."; \
		exit 1; \
	else \
		echo "All files are properly formatted."; \
	fi

# Fix code formatting
fmt-fix:
	@echo "==> Fixing code formatting..."
	@gofmt -w .
	@echo "Formatting complete."

# Run all checks (same as CI)
check: fmt vet build test
	@echo "\n==> All checks passed! ✓"

# Alias for check
ci: check

# Clean build artifacts and coverage files
clean:
	@echo "==> Cleaning build artifacts..."
	@rm -f coverage.out coverage.txt coverage.html
	@rm -f examples/*/sqlite-mount examples/*/ragfs-mount
	@echo "Clean complete."

# Help target
help:
	@echo "Available targets:"
	@echo "  make test      - Run tests with race detection and coverage"
	@echo "  make build     - Build all packages"
	@echo "  make vet       - Run go vet"
	@echo "  make fmt       - Check code formatting"
	@echo "  make fmt-fix   - Fix code formatting issues"
	@echo "  make check     - Run all checks (fmt, vet, build, test)"
	@echo "  make ci        - Alias for 'make check'"
	@echo "  make clean     - Remove build artifacts and coverage files"
	@echo "  make help      - Show this help message"
