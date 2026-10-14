#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
A="${HF_M5_A:-hugofleet-a}"
B="${HF_M5_B:-hugofleet-b}"
case "$A:$B" in hugofleet-a:hugofleet-b|hugofleet-m5-e2e-a:hugofleet-m5-e2e-b) ;; *) fail 'Unsupported M5 Worker pair.' ;; esac
STATE="$(realpath -m "${HF_M5_STATE:-$ROOT/.local/tutorials/m5}")"
case "$STATE" in "$ROOT"/.local/tutorials/m5|"$ROOT"/.local/tutorials/m5/*|"$ROOT"/.local/m5|"$ROOT"/.local/m5-*) ;; *) fail 'M5 state must remain in its project runtime folders.' ;; esac
export HF_LEARNING_ROOT="$STATE"
HUB="${HF_M5_HUB:-hugofleet-hub-m5}"
DEX="${HF_M5_DEX:-hugofleet-dex-m5}"
case "$HUB" in hugofleet-hub-m5|hugofleet-hub-m5-e2e|hugofleet-hub-m5-development) ;; *) fail 'Unsupported M5 Hub.' ;; esac
case "$DEX" in hugofleet-dex-m5|hugofleet-dex-m5-e2e|hugofleet-dex-m5-development) ;; *) fail 'Unsupported M5 Dex.' ;; esac
mkdir -p "$STATE"
[[ "$(id -u)" != 0 ]] || fail 'Run M5 as a non-root Linux user.'
owned_container(){
 local name="$1" record="$2"
 [[ -f "$record" ]] || fail "$name has no ownership record."
 [[ "$(docker inspect --format '{{.Id}}' "$name")" == "$(cat "$record")" ]] || fail "$name ownership mismatch."
}
wait_http(){
 local url="$1"
 for ((i=0;i<60;i++));do if curl -fsS --max-time 2 "$url" >/dev/null 2>&1;then return;fi;sleep 1;done
 fail "Service did not become healthy: $url"
}
dex_up(){
 if [[ ! -f "$STATE/accounts.json" ]];then go run ./stages/m4/cmd/demo-config --output "$STATE" --issuer=http://127.0.0.1:5557/dex --base-url=http://127.0.0.1:18095;fi
 if docker inspect "$DEX" >/dev/null 2>&1;then
  owned_container "$DEX" "$STATE/dex.id"
  [[ "$(docker inspect --format '{{.State.Running}}' "$DEX")" == true ]] || docker start "$DEX" >/dev/null
 else
  timeout --foreground 1800 docker pull "$DEX_IMAGE"
  docker create --name "$DEX" --network kind --read-only --cap-drop ALL --security-opt no-new-privileges \
   --user "$(id -u):$(id -g)" --cpus=.5 --memory=256m --pids-limit=128 \
   --tmpfs /tmp:rw,noexec,nosuid,size=16m --entrypoint /usr/local/bin/dex -e DEX_EXPAND_ENV=false \
   -p 127.0.0.1:5557:5556 --mount "type=bind,source=$STATE/dex/config.yaml,target=/config.yaml,readonly" \
   "$DEX_IMAGE" serve /config.yaml > "$STATE/dex.id.tmp"
  docker inspect --format '{{.Id}}' "$DEX" > "$STATE/dex.id"
  rm -f "$STATE/dex.id.tmp"
  docker start "$DEX" >/dev/null
 fi
 wait_http http://127.0.0.1:5557/dex/.well-known/openid-configuration
 if [[ ! -f "$STATE/subjects.json" ]];then go run ./stages/m4/cmd/oidc-check --settings "$STATE/accounts.json" --output "$STATE/subjects.json";fi
}
hub_up(){
 owned_container "$DEX" "$STATE/dex.id"
 if docker inspect "$HUB" >/dev/null 2>&1;then
  owned_container "$HUB" "$STATE/hub.id"
  [[ "$(docker inspect --format '{{.State.Running}}' "$HUB")" == true ]] || docker start "$HUB" >/dev/null
 else
  local gateway dex_ip
  gateway="$(kind_ipv4_gateway)"
  dex_ip="$(docker inspect --format '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "$DEX")"
  go run ./stages/m4/cmd/bootstrap-hub --stage=m5 --state "$STATE" --clusters "$A,$B" --issuer-dial "$dex_ip:5556" --http-client-ip "$gateway"
  mkdir -p "$STATE/db";chmod 0700 "$STATE/db" "$STATE/certs"
  docker create --name "$HUB" --network kind --read-only --cap-drop ALL --security-opt no-new-privileges \
   --user "$(id -u):$(id -g)" --cpus=2 --memory=512m --pids-limit=128 \
   -p 127.0.0.1:18095:18095 --mount "type=bind,source=$STATE/db,target=/data" \
   --mount "type=bind,source=$STATE/hub-config.json,target=/config.json,readonly" \
   --mount "type=bind,source=$STATE/certs/ca.crt,target=/certs/ca.crt,readonly" \
   --mount "type=bind,source=$STATE/certs/hub.crt,target=/certs/hub.crt,readonly" \
   --mount "type=bind,source=$STATE/certs/hub.key,target=/certs/hub.key,readonly" \
   hugofleet-hub:m5 --listen=0.0.0.0:18095 --grpc-listen=:18443 --database=/data/hub.db \
   --certs=/certs --config=/config.json --heartbeat-timeout="${HF_M5_HEARTBEAT_TIMEOUT:-15s}" --operation-timeout="${HF_M5_OPERATION_TIMEOUT:-180s}" \
   --session-ttl="${HF_M5_SESSION_TTL:-1h}" > "$STATE/hub.id.tmp"
  docker inspect --format '{{.Id}}' "$HUB" > "$STATE/hub.id"
  rm -f "$STATE/hub.id.tmp"
  docker start "$HUB" >/dev/null
 fi
 wait_http http://127.0.0.1:18095/healthz
}
workers(){
 owned_container "$HUB" "$STATE/hub.id"
 need go;need helm;mkdir -p "$ROOT/bin"
 CGO_ENABLED=0 go build -trimpath -o "$ROOT/bin/bootstrap-worker" ./stages/m4/cmd/bootstrap-worker
 local ip="$(docker inspect --format '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "$HUB")"
 for id in "$A" "$B";do
  export HF_CLUSTER="$id";source "$ROOT/scripts/common.sh";assert_owned
  timeout --foreground 300 kind load docker-image hugofleet-operator:m1 hugofleet-agent:m5 hugofleet-netprobe:m4 --name "$id"
  for tenant in team-a team-b;do
   "$ROOT/bin/bootstrap-worker" --stage=m5 --cluster "$id" --kubeconfig "$KUBECONFIG" --action tenant --tenant "$tenant"
  done
  "$ROOT/bin/bootstrap-worker" --stage=m5 --cluster "$id" --kubeconfig "$KUBECONFIG" --action agent --tenants team-a,team-b --hub "$ip:18443" --certs "$STATE/certs"
  printf '{"cluster":"%s","hubEndpoint":"%s:18443","tenants":["team-a","team-b"],"rollout":1}\n' "$id" "$ip" > "$STATE/$id-values.json"
  chmod 0600 "$STATE/$id-values.json"
  helm upgrade --install hugofleet-worker stages/m5/chart --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$id" --values "$STATE/$id-values.json" --wait=watcher --timeout=5m
  for tenant in team-a team-b;do
   kube -n "hf-$tenant" rollout status deployment/hugofleet-operator --timeout=180s
  done
  kube -n hf-system rollout status deployment/hugofleet-agent --timeout=180s
  for tenant in team-a team-b;do
   "$ROOT/bin/bootstrap-worker" --stage=m5 --cluster "$id" --kubeconfig "$KUBECONFIG" --action receipt --tenant "$tenant"
  done
 done
 wait_http http://127.0.0.1:18095/readyz
}
reconnect_workers(){
 owned_container "$HUB" "$STATE/hub.id"
 need helm
 local ip="$(docker inspect --format '{{(index .NetworkSettings.Networks "kind").IPAddress}}' "$HUB")"
 for id in "$A" "$B";do
  export HF_CLUSTER="$id";source "$ROOT/scripts/common.sh";assert_owned
  helm upgrade hugofleet-worker stages/m5/chart --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$id" --reset-then-reuse-values --set-string "hubEndpoint=$ip:18443" --wait=watcher --timeout=5m
  helm get values hugofleet-worker --namespace hf-system --kubeconfig "$KUBECONFIG" --kube-context "kind-$id" -o json > "$STATE/$id-values.json.tmp"
  chmod 0600 "$STATE/$id-values.json.tmp"
  mv "$STATE/$id-values.json.tmp" "$STATE/$id-values.json"
  kube -n hf-system rollout status deployment/hugofleet-agent --timeout=180s
 done
 wait_http http://127.0.0.1:18095/readyz
}
case "${1:-up}" in
 build)
  need go;mkdir -p bin
  CGO_ENABLED=0 go build -trimpath -o bin/m5-hub ./stages/m5/cmd/hub
  CGO_ENABLED=0 go build -trimpath -o bin/m5-traceview ./stages/m5/cmd/traceview
  CGO_ENABLED=0 go build -trimpath -o bin/bootstrap-worker ./stages/m4/cmd/bootstrap-worker
  CGO_ENABLED=0 go build -trimpath -o bin/netprobe ./stages/m4/cmd/netprobe
  ;;
 test) go test -count=1 ./stages/m4/httpapi ./stages/m5/... ;;
 image-build)
  bash stages/m0/scripts/control.sh image-build
  bash stages/m1/scripts/control.sh image-build
  timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m5/build/hub.Dockerfile -t hugofleet-hub:m5 .
  timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m3/build/agent.Dockerfile -t hugofleet-agent:m5 .
  timeout --foreground 1800 docker build --platform linux/amd64 -f stages/m4/build/netprobe.Dockerfile -t hugofleet-netprobe:m4 .
  ;;
 up)
  require_versions;need docker;bash stages/m5/scripts/tools.sh
  bash stages/m5/scripts/control.sh image-build
  HF_CLUSTER="$A" HF_CNI_PROFILE=calico bash scripts/kind.sh up
  HF_CLUSTER="$B" HF_CNI_PROFILE=kindnet bash scripts/kind.sh up
  for id in "$A" "$B";do HF_CLUSTER="$id" bash stages/m0/scripts/control.sh image-load;done
  dex_up;hub_up;workers
  go run ./stages/m4/cmd/demo-users --settings "$STATE/accounts.json"
  printf 'M5 actual Workers connected. Open http://127.0.0.1:18095/login; private generated accounts: %s/accounts.json\n' "$STATE"
  ;;
 dex-up) dex_up ;;
 workers) workers ;;
 hub-up) dex_up;hub_up ;;
 hub-stop) owned_container "$HUB" "$STATE/hub.id";docker stop --time 15 "$HUB" ;;
 hub-recreate)
  owned_container "$HUB" "$STATE/hub.id";docker stop --time 15 "$HUB";docker rm "$HUB";rm -f "$STATE/hub.id"
  hub_up;reconnect_workers
  ;;
 status)
  owned_container "$DEX" "$STATE/dex.id";owned_container "$HUB" "$STATE/hub.id"
  docker inspect --format '{{.Name}} running={{.State.Running}}' "$DEX" "$HUB"
  for id in "$A" "$B";do export HF_CLUSTER="$id";source "$ROOT/scripts/common.sh";assert_owned;kube get nodes;kube -n hf-system get deployment hugofleet-agent;done
  ;;
 forward)
  [[ "${HF_CLUSTER:-$A}" == "$A" || "${HF_CLUSTER:-$A}" == "$B" ]] || fail 'Select this stage Worker.'
  export HF_CLUSTER="${HF_CLUSTER:-$A}";source "$ROOT/scripts/common.sh";assert_owned
  [[ "${HF_TENANT:-team-a}" == team-a || "${HF_TENANT:-team-a}" == team-b ]] || fail 'Select a provisioned Demo tenant.'
  [[ "${HF_APP:-pricing-api}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] || fail 'Application name must be DNS label.'
  exec kubectl --kubeconfig "$KUBECONFIG" --context "kind-$CLUSTER" -n "hf-${HF_TENANT:-team-a}" port-forward "service/${HF_APP:-pricing-api}" 18080:8080 --address=127.0.0.1
  ;;
 down)
  for name in "$HUB" "$DEX";do
   record="$STATE/hub.id";[[ "$name" != "$DEX" ]] || record="$STATE/dex.id"
   if docker inspect "$name" >/dev/null 2>&1;then owned_container "$name" "$record";docker stop --time 15 "$name";docker rm "$name";rm -f "$record";fi
  done
  for id in "$A" "$B";do HF_CLUSTER="$id" bash scripts/kind.sh down;done
  printf 'Owned M5 containers/Workers removed. Private DB, accounts, subjects and credentials retained: %s\n' "$STATE"
  ;;
 *) fail 'Unknown M5 action.' ;;
esac
