BINARY := proxmops
PKG := ./...

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

MODULE  := github.com/prop4n/proxmops
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION) \
           -X $(MODULE)/internal/version.Commit=$(COMMIT) \
           -X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test vet lint fmt tidy run docker snapshot clean

build: ## Build the binary into bin/
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/proxmops

test: ## Run the test suite
	go test -race $(PKG)

vet: ## Run go vet
	go vet $(PKG)

lint: ## Run golangci-lint (must be installed)
	golangci-lint run

fmt: ## Format the code
	gofmt -w .

tidy: ## Tidy module dependencies
	go mod tidy

run: ## Run the daemon
	go run ./cmd/proxmops daemon

docker: ## Build the container image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t $(BINARY):$(VERSION) .

snapshot: ## Build a local release snapshot (needs goreleaser)
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf bin dist
