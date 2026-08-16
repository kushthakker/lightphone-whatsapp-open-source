#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
	echo "usage: $0 https://your-public-bridge.example" >&2
	exit 64
}

[[ $# -eq 1 ]] || usage
public_base_url="${1%/}"
[[ "$public_base_url" == https://* ]] || {
	echo "error: the public bridge URL must use https://" >&2
	exit 1
}
[[ ! -e .env ]] || {
	echo "error: .env already exists; refusing to overwrite it" >&2
	exit 1
}
command -v openssl >/dev/null || {
	echo "error: openssl is required" >&2
	exit 1
}

domain="${public_base_url#https://}"
[[ "$domain" != */* && "$domain" != *\?* && "$domain" != *\#* ]] || {
	echo "error: use an origin URL without a path, query, or fragment" >&2
	exit 1
}
[[ "$domain" =~ ^[A-Za-z0-9.-]+(:[0-9]{1,5})?$ && "$domain" != .* && "$domain" != *. && "$domain" != *..* ]] || {
	echo "error: use a valid hostname with an optional numeric port" >&2
	exit 1
}

umask 077
api_token="$(openssl rand -hex 32)"
setup_token="$(openssl rand -hex 32)"

sed \
	-e "s|^PUBLIC_BASE_URL=.*|PUBLIC_BASE_URL=${public_base_url}|" \
	-e "s|^API_TOKEN=.*|API_TOKEN=${api_token}|" \
	-e "s|^SETUP_TOKEN=.*|SETUP_TOKEN=${setup_token}|" \
	-e "s|^DOMAIN=.*|DOMAIN=${domain}|" \
	.env.example > .env
chmod 600 .env

echo "Created .env with independent API and setup tokens."
echo "Setup URL: ${public_base_url}/setup?token=${setup_token}"
echo "Keep this output private."
