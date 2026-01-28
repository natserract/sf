-include .env

export CGO_ENABLED ?= 1
export GOOS = $(shell go env GOOS)

GOTESTSUM_VERSION = 1.9.0
GOLANGCI_VERSION = 1.62.0
BASE_DIRS = .
ROOT_DIR := $(shell pwd)
