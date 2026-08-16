#!/usr/bin/env bash
set -Eeuo pipefail

root="${1:-.}"
fail=0

report_files() {
	local label="$1"
	shift
	local matches
	matches="$(rg -l --hidden --glob '!.git/**' --glob '!dist/**' --glob '!out/**' "$@" "$root" 2>/dev/null || true)"
	if [[ -n "$matches" ]]; then
		echo "privacy scan: ${label}" >&2
		printf '%s\n' "$matches" >&2
		fail=1
	fi
}

report_files "private key material" '-----BEGIN( [A-Z0-9]+)? PRIVATE KEY-----'
report_files "AWS access key" 'AKIA[0-9A-Z]{16}'
report_files "literal bearer credential" '(?i)authorization[[:space:]]*[:=][[:space:]]*bearer[[:space:]]+[A-Za-z0-9._~+/-]{20,}'
report_files "literal token assignment" '(?i)(api|setup|access|refresh|tunnel)[_-]?token[[:space:]]*[:=][[:space:]]*[A-Za-z0-9._~+/-]{32,}'
report_files "hard-coded tool network endpoint" '\.(get|post|put|delete|patch)\([[:space:]]*"https?://'

if [[ -n "${PRIVATE_DENYLIST_FILE:-}" ]]; then
	[[ -f "$PRIVATE_DENYLIST_FILE" ]] || {
		echo "privacy scan: PRIVATE_DENYLIST_FILE does not exist" >&2
		exit 1
	}
	while IFS= read -r value; do
		[[ -z "$value" || "$value" == \#* ]] && continue
		matches="$(rg -l -F --hidden --glob '!.git/**' --glob '!dist/**' --glob '!out/**' -- "$value" "$root" 2>/dev/null || true)"
		if [[ -n "$matches" ]]; then
			echo "privacy scan: private denylist match" >&2
			printf '%s\n' "$matches" >&2
			fail=1
		fi
	done < "$PRIVATE_DENYLIST_FILE"
fi

if find "$root" -type f \( -name '*.db' -o -name '*.db-*' -o -name '*.jks' -o -name '*.keystore' -o -name '*.apk' -o -name '.env' \) -print -quit | grep -q .; then
	echo "privacy scan: forbidden runtime or signing artifact found" >&2
	find "$root" -type f \( -name '*.db' -o -name '*.db-*' -o -name '*.jks' -o -name '*.keystore' -o -name '*.apk' -o -name '.env' \) -print >&2
	fail=1
fi

[[ "$fail" -eq 0 ]] || exit 1
echo "privacy scan: clean"
