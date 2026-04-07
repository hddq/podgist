#!/usr/bin/env sh

set -eu

go_version="$(sed -n 's/^go //p' go.mod | head -n1)"

case "$go_version" in
  *.*)
    printf '%s\n' "$go_version" | cut -d. -f1,2
    ;;
  *)
    printf 'failed to parse Go version from go.mod\n' >&2
    exit 1
    ;;
esac
