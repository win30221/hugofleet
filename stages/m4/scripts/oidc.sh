#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
require_versions;need docker
export HF_M4_STATE="$ROOT/.local/m4-oidc-$(date +%s)-$$"
logs="$ROOT/artifacts/m4-oidc/$(date +%Y%m%dT%H%M%S)-$$"
mkdir -p "$logs" "$ROOT/bin"
exec > >(tee "$logs/run.log") 2>&1
name=hugofleet-dex-m4-oidc
cid="";hub_pid=""
if docker inspect "$name" >/dev/null 2>&1;then fail 'OIDC Dex already exists; inspect its original ownership record before cleanup.';fi
cleanup(){
 local result=$?
 trap - EXIT
 if [[ -n "$hub_pid" ]];then kill -TERM "$hub_pid" 2>/dev/null || true;wait "$hub_pid" || true;fi
 if [[ -n "$cid" ]];then
  docker logs "$cid" > "$logs/dex.log" 2>&1 || true
  if ! docker rm -f "$cid" >/dev/null;then result=1;fi
 fi
 [[ "$result" != 0 ]] || printf 'PASS real OIDC negatives, natural JWT/session expiry and owned cleanup; artifacts=%s\n' "$logs"
 exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
go run ./stages/m4/cmd/demo-config --output "$HF_M4_STATE"
timeout --foreground 1800 docker pull "$DEX_IMAGE"
cid="$(docker create --name "$name" --read-only --cap-drop ALL --security-opt no-new-privileges \
 --user "$(id -u):$(id -g)" --cpus=.5 --memory=256m --pids-limit=128 \
 --tmpfs /tmp:rw,noexec,nosuid,size=16m --entrypoint /usr/local/bin/dex -e DEX_EXPAND_ENV=false \
 -p 127.0.0.1:5556:5556 --mount "type=bind,source=$HF_M4_STATE/dex/config.yaml,target=/config.yaml,readonly" \
 "$DEX_IMAGE" serve /config.yaml)"
printf '%s\n' "$cid" > "$HF_M4_STATE/dex.id"
docker start "$cid" >/dev/null
ready=0
for ((i=0;i<60;i++));do
 if curl -fsS --max-time 2 http://127.0.0.1:5556/dex/.well-known/openid-configuration >/dev/null 2>&1;then ready=1;break;fi
 sleep 1
done
[[ "$ready" == 1 ]] || fail 'OIDC Dex did not become healthy.'
go run ./stages/m4/cmd/oidc-check --settings "$HF_M4_STATE/accounts.json" --output "$HF_M4_STATE/subjects.json"
go run ./stages/m4/cmd/bootstrap-hub --state "$HF_M4_STATE"
CGO_ENABLED=0 go build -trimpath -o "$ROOT/bin/m4-oidc-hub" ./stages/m4/cmd/hub
"$ROOT/bin/m4-oidc-hub" --database="$HF_M4_STATE/hub.db" --config="$HF_M4_STATE/hub-config.json" \
 --certs="$HF_M4_STATE/certs" --grpc-listen=127.0.0.1:18444 --session-ttl=5s > "$logs/hub.log" 2>&1 &
hub_pid=$!
ready=0
for ((i=0;i<60;i++));do
 kill -0 "$hub_pid" 2>/dev/null || fail 'Independent OIDC Hub exited.'
 if curl -fsS --max-time 2 http://127.0.0.1:18094/healthz >/dev/null 2>&1;then ready=1;break;fi
 sleep 1
done
[[ "$ready" == 1 ]] || fail 'OIDC Hub did not become healthy.'
go test -v -count=1 -tags=oidcintegration -timeout=22m ./stages/m4/test/oidc
