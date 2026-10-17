#!/bin/sh
# Prints the commit messages that make up a release as a Markdown list.
#
#   tools/build/release_notes.sh [REF]     commits since the previous release tag up to REF (default HEAD)
#
# The previous release tag is the newest vMAJOR.MINOR.PATCH tag before REF (REF itself is skipped when it is
# such a tag); without one, the whole history is listed.
set -e
ref=${1:-HEAD}
prev=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' "$ref^" 2>/dev/null || true)
if [ -n "$prev" ]; then
  echo "Changes since $prev:"
  echo
  git log --no-merges --pretty='- %s (%h)' "$prev..$ref"
else
  echo "Changes:"
  echo
  git log --no-merges --pretty='- %s (%h)' "$ref"
fi
