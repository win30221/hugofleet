#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"
need mktemp; need tar
mkdir -p "$ROOT/.local"
if [[ "$CACHE_ROOT" != "$ROOT/.cache" ]]; then
  native_cache="$CACHE_ROOT"
else
  # /tmp can be a small RAM disk in WSL; use the Linux disk for large Go builds.
  native_cache="$(mktemp -d /var/tmp/hugofleet-go-cache.XXXXXXXX)"
  printf '%s\n' "$ROOT" > "$native_cache/.hugofleet-root"
  mkdir -p "$native_cache/go-mod/cache/download" "$native_cache/go-build" "$native_cache/go-race" "$native_cache/go-tmp"
  if [[ -d "$GOMODCACHE/cache/download" ]]; then
    # Copy verified archives only, then Go unpacks them on Linux; sources stay in the workspace.
    tar -C "$GOMODCACHE/cache/download" -cf - . | tar -C "$native_cache/go-mod/cache/download" -xf -
  fi
fi
if [[ ! -x "$native_cache/go/bin/go" ]]; then
  archive="$ROOT/.tools/downloads/go.tar.gz"
  printf '%s  %s\n' "$GO_SHA256" "$archive" | sha256sum --check --status || fail 'Run make tools first: local Go SDK archive checksum mismatch.'
  tar -xzf "$archive" -C "$native_cache"
fi
printf '%s\n' "$native_cache" > "$ROOT/.local/native-cache.path"
printf 'Project-owned Linux cache prepared: %s\n' "$native_cache"
