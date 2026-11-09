#!/bin/sh
# Prints the CHANGELOG.md section for a version, without its heading or the
# blank lines around it. Fails when the section is missing or empty, so a
# release never goes out with empty notes.
#
#   scripts/release-notes.sh v0.2.2
#
# CI's release job uses it for the GitHub release body. Plain POSIX sh and awk,
# so it behaves the same on macOS and on GitHub's Ubuntu runners.
set -eu

version=${1:?usage: scripts/release-notes.sh vX.Y.Z}
changelog=$(dirname "$0")/../CHANGELOG.md

notes=$(awk -v head="## $version" '
  $0 == head { found = 1; next }
  found && /^## / { exit }
  found { lines[++n] = $0 }
  END {
    s = 1; while (s <= n && lines[s] ~ /^[[:space:]]*$/) s++
    e = n; while (e >= s && lines[e] ~ /^[[:space:]]*$/) e--
    for (i = s; i <= e; i++) print lines[i]
  }
' "$changelog")

if [ -z "$notes" ]; then
  echo "release-notes: CHANGELOG.md has no entries under \"## $version\"" >&2
  exit 1
fi
printf '%s\n' "$notes"
