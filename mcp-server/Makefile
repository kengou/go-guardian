# go-guardian MCP server — build targets
#
# Local binary:
#   make build
#
# Single-platform image (current machine):
#   make docker-build
#
# Multi-platform image + push to registry:
#   REGISTRY=ghcr.io/myorg make docker-push-multi
#
# Variables can be overridden on the command line:
#   IMAGE_TAG=v1.2.3 REGISTRY=ghcr.io/myorg make docker-push-multi

IMAGE_NAME ?= go-guardian-mcp
IMAGE_TAG  ?= latest
REGISTRY   ?=
PLATFORMS  ?= linux/amd64,linux/arm64

# Full image reference — omit registry prefix when REGISTRY is unset.
ifeq ($(REGISTRY),)
IMAGE := $(IMAGE_NAME):$(IMAGE_TAG)
else
IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
endif

BINARY := go-guardian-mcp
GO     := go
GOFLAGS := -ldflags="-s -w" -trimpath

.PHONY: all build test lint docker-build docker-push-multi clean help

all: build

## build: compile the binary for the current platform
build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BINARY) .

## test: run the full test suite
test:
	$(GO) test -count=1 -timeout=30s ./...

## lint: run golangci-lint (requires golangci-lint in PATH)
lint:
	golangci-lint run ./...

## docker-build: build image for the current platform only (no push)
docker-build:
	docker build \
	  --platform $$(go env GOOS)/$$(go env GOARCH) \
	  --tag $(IMAGE) \
	  .

## docker-push-multi: build multi-platform image (amd64 + arm64) and push
##   Requires: docker buildx with a builder that supports multi-platform builds.
##   Create one with: docker buildx create --use --name multi-builder
##   REGISTRY must be set: REGISTRY=ghcr.io/myorg make docker-push-multi
docker-push-multi:
ifndef REGISTRY
	$(error REGISTRY is not set. Usage: REGISTRY=ghcr.io/myorg make docker-push-multi)
endif
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --tag $(IMAGE) \
	  --push \
	  .

## docker-push-multi-local: build multi-platform image and load into local daemon
##   Note: --load only works for a single platform. Use docker-build for local testing.
docker-inspect:
	docker buildx imagetools inspect $(IMAGE)

## clean: remove build artefacts
clean:
	rm -f $(BINARY)

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
