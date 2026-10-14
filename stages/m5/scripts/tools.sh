#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
need curl;need tar;need sha256sum
[[ "$(uname -s):$(uname -m)" == Linux:x86_64 ]] || fail 'Helm tools support Linux amd64.'
mkdir -p "$ROOT/.tools/bin" "$ROOT/.tools/downloads"
archive="$ROOT/.tools/downloads/helm-$HELM_VERSION-linux-amd64.tar.gz"
if [[ ! -f "$archive" ]] || ! printf '%s  %s\n' "$HELM_SHA256" "$archive" | sha256sum --check --status;then
 curl --silent --show-error --fail --location --retry 2 --connect-timeout 15 --max-time 300 "https://get.helm.sh/helm-v$HELM_VERSION-linux-amd64.tar.gz" --output "$archive.part"
 printf '%s  %s\n' "$HELM_SHA256" "$archive.part" | sha256sum --check --status || fail 'Official Helm archive checksum mismatch.'
 mv -- "$archive.part" "$archive"
fi
tar -xzf "$archive" -O linux-amd64/helm > "$ROOT/.tools/bin/helm.tmp"
chmod 0755 "$ROOT/.tools/bin/helm.tmp";mv -- "$ROOT/.tools/bin/helm.tmp" "$ROOT/.tools/bin/helm"
[[ "$(helm version --short)" == "v$HELM_VERSION"* ]] || fail 'Helm installed version differs from pin.'
printf 'PASS project-local Helm %s official archive SHA-256 and binary version.\n' "$HELM_VERSION"
