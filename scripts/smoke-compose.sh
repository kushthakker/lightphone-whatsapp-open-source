#!/usr/bin/env bash
set -Eeuo pipefail

[[ -f .env ]] || {
	echo "error: run scripts/init-env.sh first" >&2
	exit 1
}

env_value() {
	local key="$1"
	local value
	value="$(sed -n "s/^${key}=//p" .env | tail -n 1)"
	[[ -n "$value" ]] || {
		echo "error: ${key} is missing from .env" >&2
		exit 1
	}
	printf '%s' "$value"
}

API_TOKEN="$(env_value API_TOKEN)"
SETUP_TOKEN="$(env_value SETUP_TOKEN)"
BRIDGE_PORT="$(sed -n 's/^BRIDGE_PORT=//p' .env | tail -n 1)"
SETUP_ENABLED="$(sed -n 's/^SETUP_ENABLED=//p' .env | tail -n 1)"
SETUP_ENABLED="${SETUP_ENABLED:-true}"

case "$SETUP_ENABLED" in
	true)
		setup_unauthenticated_status=401
		setup_authenticated_status=200
		;;
	false)
		setup_unauthenticated_status=404
		setup_authenticated_status=404
		;;
	*)
		echo "error: SETUP_ENABLED must be true or false" >&2
		exit 1
		;;
esac

origin="${SMOKE_ORIGIN:-http://127.0.0.1:${BRIDGE_PORT:-8080}}"

expect_status() {
	local expected="$1"
	shift
	local actual
	actual="$(curl --disable --silent --show-error --output /dev/null --write-out '%{http_code}' "$@")"
	[[ "$actual" == "$expected" ]] || {
		echo "error: expected HTTP ${expected}, got ${actual}" >&2
		exit 1
	}
}

expect_status 200 "${origin}/healthz"
expect_status "$setup_unauthenticated_status" "${origin}/setup/status"
expect_status "$setup_authenticated_status" "${origin}/setup/status?token=${SETUP_TOKEN}"
expect_status 401 "${origin}/api/v1/status"
expect_status 200 -H "Authorization: Bearer ${API_TOKEN}" "${origin}/api/v1/status"

echo "compose smoke: clean"
