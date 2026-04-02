# New Relic MCP bridge — build targets
#
# Single-platform image (current machine):
#   make docker-build
#
# Multi-platform image + push to registry:
#   REGISTRY=ghcr.io/myorg make docker-push-multi

IMAGE_NAME ?= newrelic-mcp
IMAGE_TAG  ?= latest
REGISTRY   ?=
PLATFORMS  ?= linux/amd64,linux/arm64

ifeq ($(REGISTRY),)
IMAGE := $(IMAGE_NAME):$(IMAGE_TAG)
else
IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
endif

.PHONY: docker-build docker-push-multi docker-inspect clean help

## docker-build: build image for the current platform only (no push)
docker-build:
	docker build --tag $(IMAGE) .

## docker-push-multi: build multi-platform image (amd64 + arm64) and push
docker-push-multi:
ifndef REGISTRY
	$(error REGISTRY is not set. Usage: REGISTRY=ghcr.io/myorg make docker-push-multi)
endif
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --tag $(IMAGE) \
	  --push \
	  .

## docker-inspect: show multi-platform manifest details
docker-inspect:
	docker buildx imagetools inspect $(IMAGE)

## clean: remove local image
clean:
	docker rmi $(IMAGE) 2>/dev/null || true

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
