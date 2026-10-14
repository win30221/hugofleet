#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
case "${1:-generate}" in
 tools)
  mkdir -p .tools/downloads .tools/bin
  if [[ ! -f .tools/downloads/protoc.zip ]] || ! printf '%s  %s\n' "$PROTOC_SHA256" .tools/downloads/protoc.zip | sha256sum --check --status; then
   curl -fsSL --retry 2 --connect-timeout 15 --max-time 300 "https://github.com/protocolbuffers/protobuf/releases/download/v$PROTOC_VERSION/protoc-$PROTOC_VERSION-linux-x86_64.zip" -o .tools/downloads/protoc.zip
  fi
  go run ./stages/m3/cmd/proto-tool .tools/downloads/protoc.zip "$PROTOC_SHA256" .tools/bin
  GOBIN="$ROOT/.tools/bin" go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOBUF_GO_VERSION"
  GOBIN="$ROOT/.tools/bin" go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$PROTOC_GO_GRPC_VERSION"
  ;;
 generate|check)
  need protoc; need protoc-gen-go; need protoc-gen-go-grpc
  [[ "$(protoc --version)" == "libprotoc $PROTOC_VERSION" ]] || fail 'Unexpected compiler version.'
  # Pseudo-version builds report a development version string; inspect module metadata.
  [[ "$(go version -m "$(command -v protoc-gen-go)" | awk '$1 == "mod" {print $3}')" == "$PROTOBUF_GO_VERSION" ]] || fail 'Unexpected Go protobuf generator module version.'
  [[ "$(protoc-gen-go-grpc --version)" == "protoc-gen-go-grpc $PROTOC_GO_GRPC_VERSION" ]] || fail 'Unexpected Go gRPC generator version.'
  dest="$ROOT"
  if [[ "${1:-generate}" == check ]];then
   mkdir -p .local
   dest="$(mktemp -d "$ROOT/.local/proto-check-XXXXXX")"
   trap 'case "$dest" in "$ROOT"/.local/proto-check-*) rm -rf -- "$dest" ;; esac' EXIT
  fi
  protoc --go_out="$dest" --go_opt=module=github.com/win30221/hugofleet --go-grpc_out="$dest" --go-grpc_opt=module=github.com/win30221/hugofleet stages/m3/api/proto/hugofleet/agent/v1/control.proto
  [[ "${1:-generate}" != check ]] || diff -ru stages/m3/api/agent "$dest/stages/m3/api/agent"
  ;;
 *) fail 'Unknown proto command.' ;;
esac
