#!/usr/bin/env bash
# Fixed tutorial operations; never accepts arbitrary kubectl or shell arguments.
set -Eeuo pipefail
lesson_stage="${1:-}"
lesson_worker="${2:-}"
lesson_action="${3:-}"
case "$lesson_stage" in m3|m4|m5) ;; *) printf 'Select m3, m4 or m5.\n' >&2; exit 1 ;; esac
case "$lesson_worker" in a|b) ;; *) printf 'Select Worker a or b.\n' >&2; exit 1 ;; esac
case "$lesson_action" in status|forward|drift|observe|agent-stop|agent-start) ;; *) printf 'Unsupported lesson action.\n' >&2; exit 1 ;; esac
lesson_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "$lesson_root/stages/$lesson_stage/env.sh" "$lesson_worker"
if [[ "$lesson_stage" == m3 ]]; then
  lesson_namespace=hf-demo
  lesson_agent_namespace=hf-demo
  lesson_app="${HF_APP:-pricing-$lesson_worker}"
else
  case "${HF_TENANT:-team-a}" in team-a|team-b) ;; *) fail 'Select Demo tenant team-a or team-b.' ;; esac
  lesson_namespace="hf-${HF_TENANT:-team-a}"
  lesson_agent_namespace=hf-system
  lesson_app="${HF_APP:-pricing-api}"
fi
[[ ${#lesson_app} -le 63 && "$lesson_app" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] || fail 'Application name must be a DNS label.'
assert_owned
printf '%s Worker %s (%s), namespace=%s, application=%s\n' "$lesson_stage" "$lesson_worker" "$CLUSTER" "$lesson_namespace" "$lesson_app"
case "$lesson_action" in
  status)
    kube -n "$lesson_namespace" get applications,deployments,services,pods
    kube -n "$lesson_namespace" get events --sort-by=.lastTimestamp
    kube -n "$lesson_namespace" get "application/$lesson_app" "deployment/$lesson_app" \
      -o 'custom-columns=RESOURCE:.kind,NAME:.metadata.name,DESIRED:.spec.replicas,READY:.status.readyReplicas'
    ;;
  forward)
    lesson_port=18080
    [[ "$lesson_worker" != b ]] || lesson_port=18081
    kube -n "$lesson_namespace" port-forward --address 127.0.0.1 "service/$lesson_app" "$lesson_port:8080"
    ;;
  drift)
    lesson_replicas="$(kube -n "$lesson_namespace" get application "$lesson_app" -o jsonpath='{.spec.replicas}')"
    [[ "$lesson_replicas" == 2 ]] || fail 'First publish two replicas from the Hub UI and wait for SUCCEEDED / READY.'
    printf 'Application 仍要求 2；故意把 Deployment 改為 1。接著觀察 Operator 修復。\n'
    kube -n "$lesson_namespace" scale "deployment/$lesson_app" --replicas=1
    ;;
  observe)
    printf '持續觀察 Deployment；Ctrl+C 只停止觀察。漂移練習最終應恢復 2/2。\n'
    kube -n "$lesson_namespace" get deployment "$lesson_app" -w
    ;;
  agent-stop)
    printf '暫停這個 Worker 的 Agent；當地 Operator 與既有應用繼續執行。\n'
    kube -n "$lesson_agent_namespace" scale deployment/hugofleet-agent --replicas=0
    printf '回到管理頁等待 DISCONNECTED，核對最後心跳；不要立刻恢復。\n'
    ;;
  agent-start)
    kube -n "$lesson_agent_namespace" scale deployment/hugofleet-agent --replicas=1
    kube -n "$lesson_agent_namespace" rollout status deployment/hugofleet-agent --timeout=180s
    printf 'Agent Pod 已就緒；再回管理頁等待 CONNECTED，確認通道已建立。\n'
    ;;
esac
