#!/bin/sh
set -eu

ko_bin="${KO:-ko}"
platform="${PLATFORM:-linux/amd64}"
container_cli="${CONTAINER_CLI:-docker}"
container_name="braind-bootstrap-smoke-$$"

# With podman, ko talks to the docker-compatible API socket, so point it there
# when no docker daemon is configured, e.g.:
#   CONTAINER_CLI=podman DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock make image-smoke
if [ "${container_cli}" = "podman" ] && [ -z "${DOCKER_HOST:-}" ]; then
	DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
	export DOCKER_HOST
fi

cleanup() {
	"${container_cli}" rm -f "${container_name}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

image_ref="$("${ko_bin}" build ./cmd/braind --bare --local --platform="${platform}" --tags=smoke)"

# docker accepts uid/gid directly on --tmpfs; podman cannot, and always owns
# the tmpfs as container root, so under podman make it sticky world-writable
# for the 65532 process. The nonroot assertion below is unaffected.
if [ "${container_cli}" = "podman" ]; then
	tmpfs_options="rw,noexec,nosuid,nodev,mode=1777"
else
	tmpfs_options="rw,noexec,nosuid,nodev,mode=0700,uid=65532,gid=65532"
fi

"${container_cli}" run --detach --rm \
	--name "${container_name}" \
	--read-only \
	--tmpfs "/data:${tmpfs_options}" \
	--publish 127.0.0.1::8080 \
	--env BRAIND_DATA_PATH=/data \
	--env BRAIND_VAULT_PATH=/data/vault \
	--env BRAIND_GIT_DIR=/data/git/vault.git \
	--env BRAIND_SYNC_ENABLED=false \
	--env BRAIND_GIT_BACKUP_ENABLED=false \
	--env OIDC_CONFIGURATION_URL=https://oidc.example.test/.well-known/openid-configuration \
	--env OIDC_CLIENT_ID=braind \
	--env OIDC_CLIENT_SECRET=smoke-placeholder-secret \
	--env OIDC_ADMIN_GROUP=access-brain-admin \
	--env OIDC_USER_GROUP=access-brain-user \
	--env OIDC_VIEWER_GROUP=access-brain-viewer \
	"${image_ref}" >/dev/null

host_port="$("${container_cli}" inspect --format='{{(index (index .NetworkSettings.Ports "8080/tcp") 0).HostPort}}' "${container_name}")"
attempt=0
until response="$(curl --fail --silent --show-error "http://127.0.0.1:${host_port}/healthz")"; do
	attempt=$((attempt + 1))
	if [ "${attempt}" -ge 20 ]; then
		"${container_cli}" logs "${container_name}" >&2
		exit 1
	fi
	sleep 1
done

if [ "${response}" != "ok" ]; then
	printf 'unexpected health response: %s\n' "${response}" >&2
	exit 1
fi

"${container_cli}" inspect --format='{{.Config.User}}' "${container_name}" | grep -Eq '^(nonroot|65532)(:65532)?$'
printf 'bootstrap image smoke test passed: %s\n' "${image_ref}"
