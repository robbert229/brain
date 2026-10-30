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

# Debug image: same binary on an Ubuntu base so `kubectl exec ... -- bash`
# works inside the pod. Local build only; publish with image-publish-debug.
image-debug:
	KO_CONFIG_PATH=.ko-debug.yaml $(KO) build ./cmd/braind --bare --local --platform=$(PLATFORM) --tags=$(IMAGE_TAG)-debug

image-smoke:
	KO=$(KO) PLATFORM=$(PLATFORM) CONTAINER_CLI=$(CONTAINER_CLI) ./scripts/smoke-bootstrap-image.sh

image-publish:
	@test -n "$(KO_DOCKER_REPO)" || (echo "KO_DOCKER_REPO is required" >&2; exit 2)
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build ./cmd/braind --bare --tags=$(IMAGE_TAG),latest --image-refs=image-ref.txt

# Publishes the Ubuntu-based debug image as $(IMAGE_TAG)-debug. It never
# touches `latest`, and its refs land in image-ref-debug.txt so a debug roll
# can never clobber the normal deploy's image-ref.txt.
image-publish-debug:
	@test -n "$(KO_DOCKER_REPO)" || (echo "KO_DOCKER_REPO is required" >&2; exit 2)
	KO_CONFIG_PATH=.ko-debug.yaml KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build ./cmd/braind --bare --tags=$(IMAGE_TAG)-debug --image-refs=image-ref-debug.txt
