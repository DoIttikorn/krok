#!/bin/sh
# Prepares a release on the current branch: renames "## Unreleased" in
# CHANGELOG.md to the version and prints the notes the release will carry.
# Commit the change and merge it through a pull request like any other; once
# CI passes on main, CI's release job tags that commit and creates the GitHub
# release from the same notes. Nothing is tagged or pushed from here.
#
#   scripts/release.sh v0.2.3
set -eu

version=${1:?usage: scripts/release.sh vX.Y.Z}
root=$(cd "$(dirname "$0")/.." && pwd)
changelog=$root/CHANGELOG.md

if ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "release: $version is not a version like v1.2.3" >&2
  exit 1
fi
if [ "$(git -C "$root" branch --show-current)" = main ]; then
  echo "release: prepare the release on a branch; main only changes through pull requests" >&2
  exit 1
fi
if git -C "$root" rev-parse -q --verify "refs/tags/$version" >/dev/null ||
  git -C "$root" ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1; then
  echo "release: tag $version already exists" >&2
  exit 1
fi
if grep -qx "## $version" "$changelog"; then
  echo "release: CHANGELOG.md already has a \"## $version\" section" >&2
  exit 1
fi
if ! grep -qx '## Unreleased' "$changelog"; then
  echo "release: CHANGELOG.md has no \"## Unreleased\" section to release" >&2
  exit 1
fi

before=$(mktemp)
cp "$changelog" "$before"
awk -v head="## $version" '
  !done && $0 == "## Unreleased" { print head; done = 1; next }
  { print }
' "$before" >"$changelog"

# An empty Unreleased section fails here, and CHANGELOG.md goes back as it was.
if ! "$root/scripts/release-notes.sh" "$version"; then
  cp "$before" "$changelog"
  rm -f "$before"
  exit 1
fi
rm -f "$before"
