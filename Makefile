# This Makefile builds the deltaS3 binary and provides a small end-to-end smoke
# test for the local chunk and restore workflow.
BIN := deltaS3
VERSION := 0.1.0
BUILD_DIR := bin
BIN_PATH := $(BUILD_DIR)/$(BIN)

TMP_BASE := /tmp/$(BIN)
GO_CACHE := /tmp/gocache
GO_PATH := /tmp/gopath
GO_MODCACHE := $(GO_PATH)/pkg/mod
GO_TMP := /tmp

GO_ENV := GOCACHE=$(GO_CACHE) GOPATH=$(GO_PATH) GOMODCACHE=$(GO_MODCACHE) GOTMPDIR=$(GO_TMP)
GO_LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean smoke-test

 build:
	mkdir -p $(BUILD_DIR) $(GO_CACHE) $(GO_PATH)
	env $(GO_ENV) go build $(GO_LDFLAGS) -o $(BIN_PATH) ./cmd/deltas3

clean:
	rm -rf $(BUILD_DIR) $(TMP_BASE)

smoke-test: build
	rm -rf $(TMP_BASE)
	mkdir -p $(TMP_BASE)/output
	printf 'abcdefghijklmnopqrstuvwxyz0123456789' > $(TMP_BASE)/input.bin
	$(BIN_PATH) chunk \
		-in $(TMP_BASE)/input.bin \
		-out-dir $(TMP_BASE)/output \
		-chunk-size 8 \
		-bucket demo-bucket \
		-prefix demo/prefix
	$(BIN_PATH) restore \
		-manifest $(TMP_BASE)/output/manifest.json \
		-chunks $(TMP_BASE)/output/chunks \
		-out $(TMP_BASE)/restored.bin
	diff -q $(TMP_BASE)/input.bin $(TMP_BASE)/restored.bin
