.PHONY: build link serve test vet tidy clean check help

BINARY := wadb
PKG    := ./cmd/wadb

build: ## Build the wadb binary
	go build -o $(BINARY) $(PKG)

link: build ## Build then run `wadb link` to pair via QR
	./$(BINARY) link

serve: build ## Build then run `wadb serve` (stdio MCP)
	./$(BINARY) serve

test: ## Run the hermetic test suite
	go test ./...

vet: ## Run go vet
	go vet ./...

tidy: ## Run go mod tidy
	go mod tidy

check: vet test ## Run vet and tests

clean: ## Remove built binary
	rm -f $(BINARY)

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
