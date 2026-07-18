#!/usr/bin/env sh
set -eu

tag="${1:-}"
expected="${2:-}"

fail() {
  echo "release policy: $*" >&2
  exit 1
}

if printf '%s' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-experimental\.(0|[1-9][0-9]*)$'; then
  confidence="experimental"
  prerelease="true"
  title_prefix="EXPERIMENTAL"
elif printf '%s' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-(beta|rc)\.(0|[1-9][0-9]*)$'; then
  confidence="preview"
  prerelease="true"
  title_prefix="PREVIEW"
elif printf '%s' "$tag" | grep -Eq '^v[1-9][0-9]*\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
  confidence="stable"
  prerelease="false"
  title_prefix="STABLE"
  version="${tag#v}"
  grep -Fq -- "- [x] \`V${version}\`" plans/STATUS.md || fail "${tag} is blocked until V${version} is checked in plans/STATUS.md"
else
  fail "tag must be vX.Y.Z-experimental.N, vX.Y.Z-beta.N, vX.Y.Z-rc.N, or stable v1.0.0+"
fi

if [ -n "$expected" ] && [ "$expected" != "$confidence" ]; then
  fail "tag implies ${confidence}, but workflow input requested ${expected}"
fi

echo "release confidence: ${confidence} (${tag})"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "tag=${tag}"
    echo "confidence=${confidence}"
    echo "prerelease=${prerelease}"
    echo "title_prefix=${title_prefix}"
  } >> "$GITHUB_OUTPUT"
fi
