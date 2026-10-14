# 共用 kind 環境設定

[kind/single-node.yaml](kind/single-node.yaml) 是 M0～M5 共用的單節點 kind 模板。啟動時 [scripts/kind.sh](../scripts/kind.sh) 依叢集選擇 API port，將本次生成的設定存入自己的執行資料目錄。M4／M5 的 A Worker 另外設定 disableDefaultCNI／pod subnet 並安裝固定版本、校驗 SHA-256 的 Calico；只有明確的 owned M4／M5 state 能使用此 Security Profile。

預設 etcd 使用 RAM profile；Docker node 停止／重啟後不能假設資料仍在。HF_ETCD_STORAGE 可選 memory／disk，既有本機實測範圍以 [驗收紀錄](../docs/README.md) 為準。

目前冷啟動等待為 kubeadm API 15 分鐘、kubelet runtime 10 分鐘、control-plane health 8 分鐘，kind create 整體上限 30 分鐘。這些是有界啟動等待；仍須通過 Node／CoreDNS／CNI readiness，不能據此將未開始的應用 E2E 列為通過。CoreDNS 在 Node／CNI 就緒後先等 Available，接著檢查完整 rollout；不修改 progress deadline 或 status。最近的啟動失敗與重驗見 [M4 紀錄](../docs/m4-validation.md)。

各階段專屬的 Deployment／Service、CRD／RBAC、Hub／Agent 設定已放回 [stages](../stages/README.md)，這裡只保留跨階段共用模板。

## 模板如何變成實際叢集

```text
deploy/kind/single-node.yaml
→ scripts/kind.sh：代換 API port／etcd dataDir
→ 該叢集 state 的 kind-<cluster>.yaml
→ kind create cluster
→ kubeconfig／cluster.id
```

這是 kind 建立叢集的設定，不能用 kubectl apply 安裝。日常使用各課 mN-up，由 kind.sh 呼叫模板，依固定 node image 建立單節點叢集。

| 你想改什麼 | 修改位置 |
|---|---|
| kind 節點基礎 profile | 本模板與 scripts/kind.sh，影響使用它的新叢集 |
| 切 memory／disk etcd | 新建前設定 HF_ETCD_STORAGE，不需手改生成 YAML |
| M0 的資源／探針 | stages/m0/workload.yaml |
| M1 起 Operator 產生的 workload | stages/m1/controller/workload.go／Application 配置 |
| Operator 或 Agent 安裝資源 | stages/m1/config／stages/m3/config |
| M4 tenant／Agent 固定安全資源 | stages/m4/bootstrap/provision.go；由本地管理員工具建立 |

生成的 kind YAML 供診斷，正式來源是此模板。已存在叢集的 up 不會因模板修改自動重建 node。disk profile 不提供 kind down 後的自動恢復，也不能替代 Hub DB／UID 與 Worker 的一致性流程。
