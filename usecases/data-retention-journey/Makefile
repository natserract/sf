BINARY     = sfmc-retention
BUILD_DIR  = ./dist
CMD        = ./cmd

.PHONY: build build-all clean run-dry help

## build: Build for current OS/arch
build:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) $(CMD)

## build-all: Cross-compile for Linux, macOS (arm64 + amd64), Windows
build-all:
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-amd64   $(CMD)
	GOOS=linux   GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-arm64   $(CMD)
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64  $(CMD)
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64  $(CMD)
	GOOS=windows GOARCH=amd64  go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-windows.exe   $(CMD)

## run-dry: Dry-run against real SFMC (no writes). Reads creds from env vars.
run-dry: build
	$(BUILD_DIR)/$(BINARY) run \
		--jb-host  "$$SFMC_JB_HOST" \
		--jb-cookie "$$SFMC_JB_COOKIE" \
		--jb-csrf  "$$SFMC_JB_CSRF" \
		--mc-host  "$$SFMC_MC_HOST" \
		--mc-cookie "$$SFMC_MC_COOKIE" \
		--mc-csrf  "$$SFMC_MC_CSRF" \
		--bearer   "$$SFMC_BEARER" \
		--dry-run --verbose

clean:
	rm -rf $(BUILD_DIR)

help:
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'
