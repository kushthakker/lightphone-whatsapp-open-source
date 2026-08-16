#!/usr/bin/env bash
set -Eeuo pipefail

[[ $# -eq 1 && -f "$1" ]] || {
	echo "usage: $0 path/to/app.apk" >&2
	exit 64
}

apk="$1"
workspace="$(mktemp -d -t lp3-whatsapp-apk-scan.XXXXXX)"
trap 'rm -rf "$workspace"' EXIT
unzip -oq "$apk" -d "$workspace"

if find "$workspace" -type f -print0 | xargs -0 strings 2>/dev/null | rg -l \
	'AKIA[0-9A-Z]{16}|-----BEGIN( [A-Z0-9]+)? PRIVATE KEY-----|https?://[^[:space:]]+/(setup|api/v1)' >/dev/null; then
	echo "error: APK contains a credential, key, or deployment-specific bridge URL" >&2
	exit 1
fi

if [[ -n "${PRIVATE_DENYLIST_FILE:-}" ]]; then
	while IFS= read -r value; do
		[[ -z "$value" || "$value" == \#* ]] && continue
		# OkHttp's bundled public-suffix database contains arbitrary public domain
		# labels and causes false positives for short private deny-list terms.
		if find "$workspace" -type f ! -path '*/assets/PublicSuffixDatabase.list' -print0 |
			xargs -0 strings 2>/dev/null | rg -F -- "$value" >/dev/null; then
			echo "error: APK contains a private denylist value" >&2
			exit 1
		fi
	done < "$PRIVATE_DENYLIST_FILE"
fi

echo "APK scan: clean"
