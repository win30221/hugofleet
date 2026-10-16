#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/../../../scripts/common.sh"
[[ "$CLUSTER" == hugofleet-m1-demo ]] || fail 'Use make m1-<action> for the M1 learning cluster.'
assert_owned

case "${1:-}" in
  update-good|break|restore)
    case "$1" in
      update-good) previous=1; revision=2; image=v2-good; condition=Ready ;;
      break) previous=2; revision=3; image=v2-broken; condition=Degraded ;;
      restore) previous=3; revision=4; image=v1; condition=Ready ;;
    esac
    current="$(kube -n hf-demo get application pricing-api -o jsonpath='{.spec.revision}')"
    [[ "$current" == "$previous" ]] || fail "This lesson step requires revision $previous; current revision is $current. Run make m1-status and follow the README custom-revision instructions."
    printf '提交 Application revision %s：映像 %s、兩個副本。\n' "$revision" "$image"
    kube -n hf-demo patch application pricing-api --type=merge \
      -p "{\"spec\":{\"revision\":$revision,\"image\":\"pricing-api:$image\",\"replicas\":2}}"
    generation="$(kube -n hf-demo get application pricing-api -o jsonpath='{.metadata.generation}')"
    kube -n hf-demo wait --for="jsonpath={.status.observedGeneration}=$generation" application/pricing-api --timeout=180s
    kube -n hf-demo wait --for="condition=$condition" application/pricing-api --timeout=180s
    kube -n hf-demo get application pricing-api -o yaml
    kube -n hf-demo get deployments,pods
    if [[ "$condition" == Degraded ]]; then
      printf '已觀測到降級；請核對目前 generation、Degraded 原因與 readiness 事件。\n'
      kube -n hf-demo get events --sort-by=.lastTimestamp
    else
      printf '請核對目前 generation 與兩個就緒副本，再重啟 make m1-forward 驗證 API。\n'
    fi
    ;;
  drift)
    replicas="$(kube -n hf-demo get application pricing-api -o jsonpath='{.spec.replicas}')"
    [[ "$replicas" == 2 ]] || fail 'Application must request two replicas; finish step 8 first.'
    printf 'Application 仍要求 2；刻意把 Deployment 改為 1，觀察 Operator 修回 2。\n'
    kube -n hf-demo patch deployment pricing-api --type=merge -p '{"spec":{"replicas":1}}'
    printf '接著執行 make m1-observe。修復可能已完成；以 make m1-status 核對兩邊設定。\n'
    ;;
  status)
    kube -n hf-demo get application pricing-api -o yaml
    printf 'Application 期望副本：'
    kube -n hf-demo get application pricing-api -o jsonpath='{.spec.replicas}{"\n"}'
    printf 'Deployment 期望副本：'
    kube -n hf-demo get deployment pricing-api -o jsonpath='{.spec.replicas}{"\n"}'
    kube -n hf-demo get deployments,services,pods
    ;;
  observe)
    printf '持續觀察 Deployment；最終 READY 應為 2/2，Ctrl+C 只停止觀察。\n'
    kube -n hf-demo get deployment pricing-api -w
    ;;
  delete)
    printf '刪除練習 Application，等待相依 Deployment／Service 清理。\n'
    kube -n hf-demo delete application pricing-api
    kube -n hf-demo wait --for=delete deployment/pricing-api --timeout=180s
    kube -n hf-demo wait --for=delete service/pricing-api --timeout=180s
    kube -n hf-demo get applications,deployments,services,replicasets,pods
    ;;
  *) fail 'Supported actions: update-good, drift, break, restore, status, observe, delete.' ;;
esac
