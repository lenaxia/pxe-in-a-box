.PHONY: build test test-short clean lint

BINARY_DIR = bin

build: $(BINARY_DIR)/pxe-gen $(BINARY_DIR)/pxe-in-a-box

$(BINARY_DIR)/pxe-gen: $(shell find . -name '*.go' -not -name '*_test.go')
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $@ ./cmd/pxe-gen

$(BINARY_DIR)/pxe-in-a-box: $(shell find . -name '*.go' -not -name '*_test.go')
	mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $@ ./cmd/pxe-in-a-box

test:
	go test ./... -v

test-short:
	go test ./... -v -short

test-fast:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf $(BINARY_DIR)

docker-build:
	docker build -t pxe-in-a-box:dev .

# Cross-compile for ARM64 (Raspberry Pi)
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o $(BINARY_DIR)/pxe-gen-arm64 ./cmd/pxe-gen
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-s -w' -o $(BINARY_DIR)/pxe-in-a-box-arm64 ./cmd/pxe-in-a-box
