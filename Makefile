## archfit — Architecture fitness checker
## Usage: make [target]

BINARY    := archfit
CMD       := ./cmd/archfit
BIN_DIR   := .bin
MODULE    := github.com/alexei-led/archfit

VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS   := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GOLANGCI_LINT_VERSION := v2.1.6
MOQ_VERSION           := v0.4.0

.DEFAULT_GOAL := help

## help: show this help message
.PHONY: help
help: ## show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## all: fmt lint test build
.PHONY: all
all: fmt lint test build ## fmt + lint + test + build

## build: compile the archfit binary
.PHONY: build
build: ## compile the archfit binary
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD)

## test: run all tests with race detector and coverage
.PHONY: test
test: ## run all tests with race detector and coverage
	go test -race -coverprofile=coverage.out ./...

## test-coverage: open HTML coverage report
.PHONY: test-coverage
test-coverage: test ## open HTML coverage report
	go tool cover -html=coverage.out

## lint: run golangci-lint
.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run -c .golangci.yaml ./...

## fmt: format Go source with gofmt and goimports
.PHONY: fmt
fmt: ## format Go source with gofmt and goimports
	gofmt -s -w .
	goimports -w -local $(MODULE) .

## mock: regenerate moq mocks via go generate
.PHONY: mock
mock: ## regenerate moq mocks via go generate
	go generate ./...

## setup-tools: install pinned development tools
.PHONY: setup-tools
setup-tools: ## install pinned development tools
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/matryer/moq@$(MOQ_VERSION)

## clean: remove build artefacts
.PHONY: clean
clean: ## remove build artefacts
	rm -rf $(BIN_DIR) coverage.out

## version: print build version info
.PHONY: version
version: ## print build version info
	@echo "version: $(VERSION)  commit: $(COMMIT)  date: $(DATE)"
