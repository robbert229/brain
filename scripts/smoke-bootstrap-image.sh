#!/bin/sh
set -eu

ko_bin="${KO:-ko}"
platform="${PLATFORM:-linux/amd64}"
container_name="braind-bootstrap-smoke-$$"

cleanup() {
	docker rm -f "${container_name}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

image_ref="$("${ko_bin}" build ./cmd/braind --bare --local --platform="${platform}" --tags=smoke)"

docker run --detach --rm \
	--name "${container_name}" \
	--read-only \
	--tmpfs /data:rw,noexec,nosuid,nodev,mode=0700,uid=65532,gid=65532 \
	--publish 127.0.0.1::8080 \
	--env BRAIND_DATA_PATH=/data \
	--env BRAIND_VAULT_PATH=/data/vault \
	--env BRAIND_GIT_DIR=/data/git/vault.git \
	--env BRAIND_SYNC_ENABLED=false \
	--env BRAIND_GIT_BACKUP_ENABLED=false \
	--env OIDC_ENABLED=false \
	"${image_ref}" >/dev/null

host_port="$(docker inspect --format='{{(index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort}}' "${container_name}")"
attempt=0
until response="$(curl --fail --silent --show-error "http://127.0.0.1:${host_port}/healthz")"; do
	attempt=$((attempt + 1))
	if [ "${attempt}" -ge 20 ]; then
		docker logs "${container_name}" >&2
		exit 1
	fi
	sleep 1
done

if [ "${response}" != "ok" ]; then
	printf 'unexpected health response: %s\n' "${response}" >&2
	exit 1
fi

docker inspect --format='{{.Config.User}}' "${container_name}" | grep -Eq '^(nonroot|65532)(:65532)?$'
printf 'bootstrap image smoke test passed: %s\n' "${image_ref}"
