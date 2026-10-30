KO ?= ko
IMAGE_TAG ?= dev
PLATFORM ?= linux/$(shell go env GOARCH)
CONTAINER_CLI ?= docker

run:
	go run ./cmd/brain

run-braind:
	go run ./cmd/braind

test:
	go test ./...

image:
	$(KO) build ./cmd/braind --bare --local --platform=$(PLATFORM) --tags=$(IMAGE_TAG)

image-smoke:
	KO=$(KO) PLATFORM=$(PLATFORM) CONTAINER_CLI=$(CONTAINER_CLI) ./scripts/smoke-bootstrap-image.sh

image-publish:
	@test -n "$(KO_DOCKER_REPO)" || (echo "KO_DOCKER_REPO is required" >&2; exit 2)
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build ./cmd/braind --bare --tags=$(IMAGE_TAG),latest --image-refs=image-ref.txt
