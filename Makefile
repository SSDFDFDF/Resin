.DEFAULT_GOAL := help

SHELL := /bin/bash

APP_NAME ?= resin
BINARY ?= $(APP_NAME)
CMD_PATH ?= ./cmd/resin
WEBUI_DIR ?= webui
GO ?= go
NPM ?= npm
DOCKER ?= docker

GO_TAGS ?= with_quic with_wireguard with_grpc with_utls
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DOCKER_IMAGE ?= $(APP_NAME):dev

LDFLAGS := -s -w \
	-X github.com/Resinat/Resin/internal/buildinfo.Version=$(VERSION) \
	-X github.com/Resinat/Resin/internal/buildinfo.GitCommit=$(GIT_COMMIT) \
	-X github.com/Resinat/Resin/internal/buildinfo.BuildTime=$(BUILD_TIME)

.PHONY: help webui-install webui-build build test clean run docker-build

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

webui-install: ## Install WebUI dependencies with npm ci
	cd $(WEBUI_DIR) && $(NPM) ci

webui-build: ## Build the embedded WebUI assets
	@if [ ! -d "$(WEBUI_DIR)/node_modules" ]; then $(MAKE) webui-install; fi
	cd $(WEBUI_DIR) && $(NPM) run build

build: webui-build ## Build the resin binary
	CGO_ENABLED=0 $(GO) build -trimpath \
		-tags '$(GO_TAGS)' \
		-ldflags '$(LDFLAGS)' \
		-o $(BINARY) $(CMD_PATH)

test: ## Run Go test suite
	$(GO) test ./...

clean: ## Remove build artifacts
	rm -rf $(BINARY) $(WEBUI_DIR)/dist

run: build ## Build and run the application
	./$(BINARY)

docker-build: ## Build the local Docker image
	$(DOCKER) build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE) .
