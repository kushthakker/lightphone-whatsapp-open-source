#!/usr/bin/env bash
set -Eeuo pipefail

readonly sdk_ref="522f94d5e862bd8824b43f3dfc76221105b720d5"
readonly sdk_url="https://github.com/lightphone/light-sdk.git"
readonly cache_dir="${LIGHT_SDK_CACHE:-.cache/light-sdk}"

for command in git rsync; do
	command -v "$command" >/dev/null || {
		echo "error: ${command} is required" >&2
		exit 1
	}
done

if ! java -version >/dev/null 2>&1; then
	if command -v brew >/dev/null && [[ -x "$(brew --prefix openjdk@17 2>/dev/null)/bin/java" ]]; then
		export JAVA_HOME="$(brew --prefix openjdk@17)/libexec/openjdk.jdk/Contents/Home"
	else
		echo "error: Java 17 is required (for Homebrew: brew install openjdk@17)" >&2
		exit 1
	fi
fi

if [[ -z "${ANDROID_HOME:-}" ]] && command -v brew >/dev/null; then
	android_home="$(brew --prefix android-commandlinetools 2>/dev/null || true)"
	if [[ -d "$android_home/platforms" ]]; then
		export ANDROID_HOME="$android_home"
	fi
fi
[[ -n "${ANDROID_HOME:-}" && -d "$ANDROID_HOME" ]] || {
	echo "error: set ANDROID_HOME to an installed Android SDK" >&2
	exit 1
}

mkdir -p "$(dirname "$cache_dir")" dist
if [[ ! -d "$cache_dir/.git" ]]; then
	git clone --filter=blob:none --no-checkout "$sdk_url" "$cache_dir"
fi
git -C "$cache_dir" fetch --no-tags --depth=1 origin "$sdk_ref"
git -C "$cache_dir" checkout --detach --force "$sdk_ref"

workspace="$(mktemp -d -t lp3-whatsapp-build.XXXXXX)"
trap 'rm -rf "$workspace"' EXIT
rsync -a --exclude='.git' --exclude='**/build' --exclude='.gradle' "$cache_dir/" "$workspace/"
rsync -a --delete tool/ "$workspace/tool/"

(
	cd "$workspace"
	./gradlew --no-daemon --build-cache :tool:testDebugUnitTest :tool:lintDebug :tool:assembleDebug
)

apk="$(find "$workspace/tool/build/outputs/apk/debug" -name '*.apk' -type f -print -quit)"
[[ -n "$apk" ]] || {
	echo "error: Gradle completed without producing a debug APK" >&2
	exit 1
}
install -m 0644 "$apk" dist/lp3-whatsapp-bridge-debug.apk
echo "Built dist/lp3-whatsapp-bridge-debug.apk"
