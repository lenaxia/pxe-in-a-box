.PHONY: build test test-unit test-integration test-e2e test-e2e-qemu test-all test-ansible clean lint docker-build download-matchbox

BINARY_DIR = bin
MATCHBOX_VERSION = v0.11.0

# ── Build ────────────────────────────────────────────────────────────

build: $(BINARY_DIR)/pxe-gen $(BINARY_DIR)/pxe-in-a-box

$(BINARY_DIR)/pxe-gen: $(shell find . -name '*.go' -not -name '*_test.go')
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $@ ./cmd/pxe-gen

$(BINARY_DIR)/pxe-in-a-box: $(shell find . -name '*.go' -not -name '*_test.go')
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $@ ./cmd/pxe-in-a-box

# ── Tests ────────────────────────────────────────────────────────────

test: test-unit

test-unit:
	go test -race -v ./...

test-integration:
	go test -tags=integration -race -v ./internal/e2e/...

test-e2e: test-e2e-http test-e2e-qemu

test-e2e-http:
	go test -tags=e2e -v ./test/e2e/... -run '^TestE2E_[^Q]' -timeout 60s

test-e2e-qemu:
	go test -tags=e2e -v ./test/e2e/... -run '^TestE2E_QEMU' -timeout 120s

test-all: test-unit test-integration test-e2e
	@echo "=== All tests passed ==="

# ── Lint ─────────────────────────────────────────────────────────────

lint:
	go vet ./...
	go vet -tags=integration ./internal/e2e/...
	go vet -tags=e2e ./test/e2e/...
	gofmt -l .

lint-full: lint
	golangci-lint run --timeout=5m || true

# ── Docker ───────────────────────────────────────────────────────────

docker-build:
	docker build -t pxe-in-a-box:dev .

# ── Dependencies ─────────────────────────────────────────────────────

download-matchbox:
	@echo "Downloading matchbox $(MATCHBOX_VERSION)..."
	curl -sL "https://github.com/poseidon/matchbox/releases/download/$(MATCHBOX_VERSION)/matchbox-$(MATCHBOX_VERSION)-linux-amd64.tar.gz" \
		| tar xz --strip-components=1 -C $(HOME)/bin matchbox-$(MATCHBOX_VERSION)-linux-amd64/matchbox
	@echo "Installed to $(HOME)/bin/matchbox"
	@matchbox -version

# ── Cross-compile ────────────────────────────────────────────────────

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o $(BINARY_DIR)/pxe-gen-arm64 ./cmd/pxe-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o $(BINARY_DIR)/pxe-in-a-box-arm64 ./cmd/pxe-in-a-box

# ── Ansible ──────────────────────────────────────────────────────────

test-ansible:
	cd ansible && ansible-playbook tests/test_templates.yml -i tests/inventory.ini
	cd ansible && ansible-playbook tests/test_playbook_syntax.yml -i tests/inventory.ini

# ── Clean ────────────────────────────────────────────────────────────

clean:
	rm -rf $(BINARY_DIR) coverage.out
