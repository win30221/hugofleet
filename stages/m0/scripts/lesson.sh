#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
[[ "$CLUSTER" == hugofleet-m0-demo ]] || fail 'Use make m0-<action> for the M0 learning cluster.'
assert_owned

case "${1:-}" in
  update-good|break|restore)
    case "$1" in
      update-good) image=v2-good ;;
      break) image=v2-broken ;;
      restore) image=v1 ;;
    esac
    printf '設定 pricing-api 映像為 %s；副本數保持目前設定。\n' "$image"
    kube -n hf-demo set image deployment/pricing-api "pricing-api=pricing-api:$image"
    if [[ "$1" == break ]]; then
      printf '已提交故障版。接著執行 make m0-status 查看 Pod，make m0-observe 持續觀察。\n'
      printf '新版 Running 但未 Ready；請依教學核對事件，不能僅憑等待逾時判定成功。\n'
    else
      kube -n hf-demo rollout status deployment/pricing-api --timeout=180s
      kube -n hf-demo get deployments,pods
      printf '更新完成；請在轉送終端按 Ctrl+C，再執行 make m0-forward。\n'
    fi
    ;;
  scale-two)
    printf '設定 Deployment 為兩個副本，等待就緒。\n'
    kube -n hf-demo scale deployment/pricing-api --replicas=2
    kube -n hf-demo rollout status deployment/pricing-api --timeout=180s
    kube -n hf-demo get deployments,pods
    ;;
  status)
    kube -n hf-demo get deployments,services,pods
    kube -n hf-demo get events --sort-by=.lastTimestamp
    ;;
  observe)
    printf '持續觀察 Deployment；Ctrl+C 只停止觀察。\n'
    kube -n hf-demo get deployment pricing-api -w
    ;;
  *) fail 'Supported actions: update-good, scale-two, break, restore, status, observe.' ;;
esac
