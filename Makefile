BINARY_NAME := gitops
TOOL_BINARY  := gitops-tool
VAULT_BINARY := vault

.PHONY: all build build-tool clean test e2e

all: build build-tool

build:
	go build -ldflags '-s -w' -o $(BINARY_NAME) ./cmd/plugin-gitops

build-tool:
	go build -ldflags '-s -w' -o $(TOOL_BINARY) ./cmd/tool

clean:
	rm -f $(BINARY_NAME) $(TOOL_BINARY)

test:
	go test ./...

e2e: build-tool
	$(VAULT_BINARY) server -dev -dev-root-token-id=root & \
	VAULT_PID=$$!; \
	sleep 3; \
	VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root ./$(TOOL_BINARY) test examples/full; EXIT=$$?; \
	kill $$VAULT_PID 2>/dev/null || true; \
	exit $$EXIT
