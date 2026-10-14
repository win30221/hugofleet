#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
need go; need curl; need timeout
mkdir -p "$ROOT/.tools/bin" "$ROOT/.tools/downloads"
export GOBIN="$ROOT/.tools/bin"
if [[ ! -x "$GOBIN/controller-gen" ]] || [[ "$(controller-gen --version)" != "Version: v$CONTROLLER_GEN_VERSION" ]]; then
  timeout --foreground 900 go install "sigs.k8s.io/controller-tools/cmd/controller-gen@v$CONTROLLER_GEN_VERSION"
fi
if [[ ! -x "$GOBIN/setup-envtest" ]] || ! go version -m "$GOBIN/setup-envtest" | grep -q "v$CONTROLLER_RUNTIME_VERSION"; then
  timeout --foreground 900 go install "sigs.k8s.io/controller-runtime/tools/setup-envtest@v$CONTROLLER_RUNTIME_VERSION"
fi
if [[ "${1:-}" == scaffold ]]; then
  file="$ROOT/.tools/downloads/kubebuilder"
  if [[ ! -f "$file" ]] || ! printf '%s  %s\n' "$KUBEBUILDER_SHA256" "$file" | sha256sum --check --status; then
    curl -fsSL --continue-at - --retry 1 --connect-timeout 15 --max-time 1200 \
      "https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$KUBEBUILDER_VERSION/kubebuilder_linux_amd64" -o "$file.part"
    printf '%s  %s\n' "$KUBEBUILDER_SHA256" "$file.part" | sha256sum --check --status || fail 'Kubebuilder checksum mismatch.'
    mv -- "$file.part" "$file"
  fi
  install -m 0755 "$file" "$GOBIN/kubebuilder"
fi
if [[ "${1:-}" == envtest ]]; then
  timeout --foreground 1800 setup-envtest use "$ENVTEST_K8S_VERSION" --bin-dir "$ROOT/.tools/envtest" -p path
fi
