#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/common.sh"
[[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'M0 tools support Linux amd64, including Ubuntu WSL2.'
need curl; need tar; need sha256sum
mkdir -p "$ROOT/.tools/bin" "$ROOT/.tools/downloads"

download() {
  local url="$1" file="$2" digest="$3"
  if [[ ! -f "$file" ]] || ! printf '%s  %s\n' "$digest" "$file" | sha256sum --check --status; then
    curl --silent --show-error --fail --location --retry 2 --connect-timeout 15 --max-time 300 "$url" --output "$file.part"
    printf '%s  %s\n' "$digest" "$file.part" | sha256sum --check --status || fail "Checksum mismatch: $file.part"
    mv -- "$file.part" "$file"
  fi
}

if [[ ! -x "$ROOT/.tools/go/bin/go" ]] || [[ "$("$ROOT/.tools/go/bin/go" version)" != "go version go$GO_VERSION linux/amd64" ]]; then
  [[ ! -d "$ROOT/.tools/go" ]] || fail 'Existing local Go differs from the pin; use a fresh .tools directory before upgrading.'
  download "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz" "$ROOT/.tools/downloads/go.tar.gz" "$GO_SHA256"
  tar -xzf "$ROOT/.tools/downloads/go.tar.gz" -C "$ROOT/.tools"
fi
if [[ "${HF_KIND_SOURCE:-0}" == 1 ]]; then
  # Officially documented source install; Go verifies module checksums.
  GOBIN="$ROOT/.tools/bin" CGO_ENABLED=0 timeout --foreground 600 go install "sigs.k8s.io/kind@v$KIND_VERSION"
  sha256sum "$ROOT/.tools/bin/kind" > "$ROOT/.tools/kind-source.sha256"
elif [[ -f "$ROOT/.tools/kind-source.sha256" ]] && sha256sum --check --status "$ROOT/.tools/kind-source.sha256" \
     && [[ "$("$ROOT/.tools/bin/kind" version)" == "kind v$KIND_VERSION "* ]]; then
  printf 'Reusing the verified project-local source build of kind %s.\n' "$KIND_VERSION"
else
  download "https://github.com/kubernetes-sigs/kind/releases/download/v$KIND_VERSION/kind-linux-amd64" "$ROOT/.tools/downloads/kind" "$KIND_SHA256"
  install -m 0755 "$ROOT/.tools/downloads/kind" "$ROOT/.tools/bin/kind"
fi
existing_kubectl="$(command -v kubectl || true)"
if [[ -n "$existing_kubectl" ]] && printf '%s  %s\n' "$KUBECTL_SHA256" "$existing_kubectl" | sha256sum --check --status; then
  install -m 0755 "$existing_kubectl" "$ROOT/.tools/downloads/kubectl"
else
  download "https://dl.k8s.io/release/v$KUBECTL_VERSION/bin/linux/amd64/kubectl" "$ROOT/.tools/downloads/kubectl" "$KUBECTL_SHA256"
fi
install -m 0755 "$ROOT/.tools/downloads/kubectl" "$ROOT/.tools/bin/kubectl"
require_versions
printf 'Pinned project-local tools installed successfully.\n'
