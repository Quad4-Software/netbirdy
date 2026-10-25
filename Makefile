.PHONY: lint lint-all lint-install setup-hooks test-unit test-privileged
GOLANGCI_LINT := $(shell pwd)/bin/golangci-lint

# Install golangci-lint locally if needed
$(GOLANGCI_LINT):
	@echo "Installing golangci-lint..."
	@mkdir -p ./bin
	@GOBIN=$(shell pwd)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Lint only changed files (fast, for pre-push)
lint: $(GOLANGCI_LINT)
	@echo "Running lint on changed files..."
	@$(GOLANGCI_LINT) run --new-from-rev=origin/main --timeout=2m

# Lint entire codebase (slow, matches CI)
lint-all: $(GOLANGCI_LINT)
	@echo "Running lint on all files..."
	@$(GOLANGCI_LINT) run --timeout=12m

# Just install the linter
lint-install: $(GOLANGCI_LINT)

# Setup git hooks for all developers
setup-hooks:
	@git config core.hooksPath .githooks
	@chmod +x .githooks/pre-push .githooks/commit-msg
	@echo "✅ Git hooks configured! Pre-push runs 'make lint'; commit-msg refuses attribution trailers"

# Host-safe unit tests: excludes the privileged-tagged tests (root / system-mutating).
# Runs as a normal user with no sudo and leaves host networking untouched.
test-unit:
	@go test -tags devcert -timeout 10m ./...

# Privileged suite: runs the `privileged`-tagged tests inside a --privileged
# --cap-add=NET_ADMIN container via the ory/dockertest harness. Requires Docker.
# Narrow the run with env vars, e.g.:
#   PRIV_RUN=TestNftablesManager PRIV_PKGS=./client/firewall/nftables/... make test-privileged
test-privileged:
	@go test -tags 'devcert privileged' -timeout 30m -run TestRunPrivilegedSuiteInDocker -v ./client/testutil/privileged/...

# NetBirdy fork targets

DASHBOARD_DIR := dashboard
PODMAN_DIR := podman
COMPOSE ?= podman compose
COMPOSE_FILE := $(PODMAN_DIR)/compose.yaml

.PHONY: build-dashboard test-dashboard podman-build podman-build-client podman-up podman-down podman-logs podman-test podman-clean

build-dashboard:
	pnpm install --frozen-lockfile && pnpm --filter netbirdy-dashboard build

test-dashboard:
	pnpm --filter netbirdy-dashboard lint && pnpm --filter netbirdy-dashboard exec tsc --noEmit && pnpm --filter netbirdy-dashboard test:unit

podman-build-client:
	podman build -f podman/Containerfile.client -t localhost/netbirdy-client:dev .

podman-build:
	podman build -f $(PODMAN_DIR)/Containerfile.server -t localhost/netbirdy-server .
	podman build -f $(PODMAN_DIR)/Containerfile.dashboard -t localhost/netbirdy-dashboard .

podman-up:
	sh $(PODMAN_DIR)/setup.sh
	$(COMPOSE) -f $(COMPOSE_FILE) up -d

podman-down:
	$(COMPOSE) -f $(COMPOSE_FILE) down

podman-logs:
	$(COMPOSE) -f $(COMPOSE_FILE) logs -f

podman-clean:
	-$(COMPOSE) -f $(COMPOSE_FILE) down -v --rmi local
	rm -f $(PODMAN_DIR)/.env $(PODMAN_DIR)/config.yaml

podman-test:
	podman run --rm \
		-v $(CURDIR):/src:ro,Z -w /src \
		-v netbirdy-go-mod:/go/pkg/mod -v netbirdy-go-build:/root/.cache/go-build \
		docker.io/library/golang:1.27.2-bookworm \
		sh -c "apt-get update -qq && apt-get install -y -qq gcc libc6-dev >/dev/null && go build ./combined/... ./client/cmd/... && go test -tags devcert ./client/cmd/... ./combined/..."
	podman run --rm \
		-v $(CURDIR):/src:ro,Z -w /src \
		-v netbirdy-pnpm-store:/pnpm-store \
		docker.io/library/node:24-bookworm \
		sh -c "npm install -g pnpm@12.14.0 -q && pnpm config set store-dir /pnpm-store && pnpm install --frozen-lockfile --filter netbirdy-dashboard... && pnpm --filter netbirdy-dashboard lint || true; pnpm --filter netbirdy-dashboard exec tsc --noEmit --tsBuildInfoFile /tmp/tsconfig.tsbuildinfo && pnpm --filter netbirdy-dashboard test:unit"

.PHONY: offline-e2e
offline-e2e:
	./scripts/offline-e2e.sh up
	./scripts/offline-e2e.sh scenario running
	./scripts/offline-e2e.sh scenario restart-one
	./scripts/offline-e2e.sh scenario restart-both
	./scripts/offline-e2e.sh scenario ip-change
	./scripts/offline-e2e.sh scenario server-back
	./scripts/offline-e2e.sh scenario revoke
	./scripts/offline-e2e.sh down

crow-lint:
	crow lint --strict .crow/

crow-exec:
	crow exec --workflow .crow/$(WORKFLOW)
