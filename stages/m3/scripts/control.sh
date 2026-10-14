#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
A="${HF_M3_A:-hugofleet-a}"
B="${HF_M3_B:-hugofleet-b}"
case "$A:$B" in hugofleet-a:hugofleet-b|hugofleet-m3-e2e-a:hugofleet-m3-e2e-b) ;; *) fail 'Unsupported M3 cluster pair.' ;; esac
STATE="$(realpath -m "${HF_M3_STATE:-$ROOT/.local/m3}")"
case "$STATE" in "$ROOT"/.local/m3|"$ROOT"/.local/m3-*|"$ROOT"/.local/tutorials/m3|"$ROOT"/.local/tutorials/m3/*) ;; *) fail 'M3 state must be in the project .local/tutorials/m3 or legacy .local/m3 folders.' ;; esac
HUB_NAME="${HF_M3_HUB:-hugofleet-hub-m3}"
case "$HUB_NAME" in hugofleet-hub-m3|hugofleet-hub-m3-e2e) ;; *) fail 'Unsupported project Hub container.' ;; esac
HTTP_PORT="${HF_M3_HTTP_PORT:-18090}"
[[ "$HTTP_PORT" =~ ^[0-9]{4,5}$ ]] && ((10#$HTTP_PORT>=1024 && 10#$HTTP_PORT<=65535)) || fail 'Invalid management port.'
HTTP_PORT="$((10#$HTTP_PORT))"
mkdir -p "$STATE"
hub_owned(){
 [[ -f "$STATE/hub.id" ]] || fail 'Hub has no project ownership record.'
 [[ "$(docker inspect --format '{{.Id}}' "$HUB_NAME")" == "$(cat "$STATE/hub.id")" ]] || fail 'Hub container ownership mismatch.'
}
certs(){
 if [[ ! -f "$STATE/certs/ca.crt" ]];then
  go run ./stages/m3/cmd/certs-dev --output "$STATE/certs" --clusters "$A,$B"
 fi
}
hub_up(){
 certs
 if docker inspect "$HUB_NAME" >/dev/null 2>&1;then
  hub_owned
  [[ "$(docker inspect --format '{{.State.Running}}' "$HUB_NAME")" == true ]] || docker start "$HUB_NAME" >/dev/null
  return
 fi
 docker network inspect kind >/dev/null
 gateway="$(kind_ipv4_gateway)"
 # Only the three explicit Hub credential files are mounted. No Worker kubeconfig,
 # client private key, CA private key, Docker socket or source tree is available.
 mkdir -p "$STATE/db"
  # Use the invoking non-root Linux UID to retain private bind-mount files.
  uid="$(id -u)";gid="$(id -g)"
  [[ "$uid" != 0 ]] || fail 'M3 requires a non-root Linux development user.'
  chmod 0700 "$STATE/db"
  chmod 0700 "$STATE/certs"
  chmod 0600 "$STATE/certs/ca.crt" "$STATE/certs/hub.crt" "$STATE/certs/hub.key" "$STATE/certs/admin-token"
 docker run -d --name "$HUB_NAME" --network kind --read-only --cap-drop ALL --security-opt no-new-privileges \
  --user "$uid:$gid" \
  --cpus=2 --memory=512m --pids-limit=128 \
  -p "127.0.0.1:$HTTP_PORT:18090" \
  --mount "type=bind,source=$STATE/db,target=/data" \
  --mount "type=bind,source=$STATE/certs/ca.crt,target=/certs/ca.crt,readonly" \
  --mount "type=bind,source=$STATE/certs/hub.crt,target=/certs/hub.crt,readonly" \
  --mount "type=bind,source=$STATE/certs/hub.key,target=/certs/hub.key,readonly" \
  --mount "type=bind,source=$STATE/certs/admin-token,target=/certs/admin-token,readonly" \
  hugofleet-hub:m3 --mode=m3 --listen=0.0.0.0:18090 --http-host="127.0.0.1:$HTTP_PORT" \
  --grpc-listen=:18443 --certs=/certs --admin-token-file=/certs/admin-token \
  --http-client-ip="$gateway" --database=/data/hub.db --policy=/policy.json --clusters="$A,$B" \
  --heartbeat-timeout="${HF_M3_HEARTBEAT_TIMEOUT:-15s}" --operation-timeout="${HF_M3_OPERATION_TIMEOUT:-180s}" > "$STATE/hub.id.tmp"
 docker inspect --format '{{.Id}}' "$HUB_NAME" > "$STATE/hub.id"
 rm -f "$STATE/hub.id.tmp"
}
agent_install(){
 hub_owned
 ip="$(docker inspect --format '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "$HUB_NAME")"
 [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Cannot resolve Hub network address.'
 for id in "$A" "$B";do
  HF_CLUSTER="$id" bash stages/m1/scripts/control.sh install
  export HF_CLUSTER="$id"; source "$ROOT/scripts/common.sh";assert_owned
  timeout --foreground 300 kind load docker-image hugofleet-agent:m3 --name "$id"
  kube -n hf-demo create secret generic hugofleet-agent-tls --from-file="ca.crt=$STATE/certs/ca.crt" \
   --from-file="$id.crt=$STATE/certs/$id.crt" --from-file="$id.key=$STATE/certs/$id.key" --dry-run=client -o yaml | kube apply -f -
  sed -e "s/HUB_ENDPOINT/$ip:18443/" -e "s/CLUSTER_ID/$id/" stages/m3/config/agent/install.yaml > "$STATE/agent-$id.yaml"
  kube apply -f "$STATE/agent-$id.yaml"
  kube -n hf-demo rollout restart deployment/hugofleet-agent
  kube -n hf-demo rollout status deployment/hugofleet-agent --timeout=180s
 done
}
case "${1:-m3-up}" in
  build)
    bash stages/m2/scripts/control.sh build
    need go; mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o bin/agent ./stages/m3/cmd/agent
    CGO_ENABLED=0 go build -trimpath -o bin/certs-dev ./stages/m3/cmd/certs-dev
    CGO_ENABLED=0 go build -trimpath -o bin/agentcheck ./stages/m3/cmd/agentcheck
    ;;
 test) need go; go test -count=1 ./stages/m3/... ;;
 certs-dev) certs ;;
 m3-hub-up) hub_up ;;
 m3-hub-stop) hub_owned; docker stop --time 15 "$HUB_NAME" ;;
 m3-hub-recreate)
  hub_owned;docker stop --time 15 "$HUB_NAME";docker rm "$HUB_NAME";rm -f "$STATE/hub.id"
  hub_up
  ;;
 agent-install) agent_install ;;
 m3-up)
  require_versions;need docker
  bash stages/m0/scripts/control.sh image-build
  bash stages/m1/scripts/control.sh image-build
  timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m2/build/hub.Dockerfile -t hugofleet-hub:m3 .
  timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m3/build/agent.Dockerfile -t hugofleet-agent:m3 .
  for id in "$A" "$B";do HF_CLUSTER="$id" bash scripts/kind.sh up; HF_CLUSTER="$id" bash stages/m0/scripts/control.sh image-load;done
  hub_up
  agent_install
  go run ./stages/m3/cmd/agentcheck --url="http://127.0.0.1:$HTTP_PORT" --token-file="$STATE/certs/admin-token" --clusters="$A,$B"
  printf 'M3 started. Open http://127.0.0.1:%s/login; private Token file: %s/certs/admin-token\n' "$HTTP_PORT" "$STATE"
  ;;
 m3-down)
  if docker inspect "$HUB_NAME" >/dev/null 2>&1;then hub_owned;docker stop --time 15 "$HUB_NAME";docker rm "$HUB_NAME";rm -f "$STATE/hub.id";fi
  for id in "$A" "$B";do HF_CLUSTER="$id" bash scripts/kind.sh down;done
  ;;
 test-e2e-m3) exec bash stages/m3/scripts/e2e.sh ;;
 *) fail 'Unknown M3 action.' ;;
esac
