.DEFAULT_GOAL := help

GO ?= go
PYTHON ?= python3
CITY ?= ../gc-management
TARGET ?= mayor
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := v2.13.2
ARGS ?=

.PHONY: help run build install test test-race test-browser test-adapters connect-mpr vet fmt fmt-check lint tools check clean

help:
	@printf '%s\n' \
	  'make run        Run the local web UI (optional ARGS="serve flags")' \
	  'make build      Build ./hold-court' \
	  'make install    Install hold-court into GOBIN or GOPATH/bin' \
	  'make test       Run Go tests' \
	  'make test-race  Run Go tests with the race detector (requires a C compiler)' \
	  'make test-browser Run live UI regression checks (requires Python Playwright)' \
	  'make test-adapters Test MPR export and consumer without external writes' \
	  'make connect-mpr Connect MPR and enable confirmed agent handoffs (CITY=... TARGET=...)' \
	  'make vet        Run go vet' \
	  'make fmt        Format Go source' \
	  'make fmt-check  Check Go formatting without changing files' \
	  'make tools      Install the CI-pinned golangci-lint (add Go bin dir to PATH)' \
	  'make lint       Run golangci-lint' \
	  'make check      Build, check formatting, vet, race-test, lint, and test adapters' \
	  'make clean      Remove the built binary; preserve feeds, rulings, and database'

run:
	$(GO) run ./cmd/hold-court serve $(ARGS)

build:
	$(GO) build -o hold-court ./cmd/hold-court

install:
	$(GO) install ./cmd/hold-court

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-browser: build
	$(PYTHON) tests/browser_live.py

test-adapters:
	$(PYTHON) -m unittest discover -s adapters/mpr -p 'test_*.py'

connect-mpr:
	$(PYTHON) adapters/mpr/install_local.py --city "$(CITY)" --target "$(TARGET)"

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

fmt-check:
	@unformatted=$$(gofmt -l cmd internal) || exit $$?; \
	if [ -n "$$unformatted" ]; then \
	  printf 'Run make fmt to format:\n%s\n' "$$unformatted"; \
	  exit 1; \
	fi

tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint:
	$(GOLANGCI_LINT) run

check: build fmt-check vet test-race lint test-adapters

clean:
	rm -f hold-court
