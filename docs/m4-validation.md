# M4 驗收範圍

M4 在 Worker 主動 mTLS 架構上驗證真實 Dex OIDC、server-side session、DB membership、租戶 namespaces／RBAC／Quota／LimitRange／NetworkPolicy 與稽核。操作見 [M4 教學](../stages/m4/README.md)，設計見 [ADR 0010](adr/0010-oidc-tenants-and-worker-bootstrap.md)，跨階段實測與 CI 證據見 [驗證總覽](validation.md)。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash，以非 root 使用者執行。先釋放教學要求的 Hub／Dex ports：

```bash
make tools operator-tools envtest proto-tools
make m4-build m4-test
make proto-check verify-generated test-integration
make test-oidc
make test-e2e-m4
```

## 已確認的行為

| 範圍 | 真實驗收內容 |
|---|---|
| OIDC／session | 六個 Dex subjects、Code + PKCE、state／nonce／browser binding、一次性登入交易、JWKS／issuer／audience／expiry、session 到期與登出 |
| 負向認證 | 缺失／錯誤 binding 消耗已知 state；重送拒絕；錯誤 verifier／redirect／JWT 拒絕；ID／access token 不充當管理 session |
| Discovery 邊界 | canonical authorization／token／JWKS wire URL、編碼與越界路徑拒絕；真實 Dex backchannel 驗證 |
| 租戶角色 | DB membership、Viewer／跨租戶／撤權拒絕、交易內授權、冪等前權限檢查、最後 Owner 保護、可信 actor 與 audit scope |
| Kubernetes 安全 | 受限 namespaced SA、租戶 allowlist／ledger 綁定、CPU／memory Pod Admission 拒絕、原因傳回 Hub／UI、新 revision 恢復及 GC |
| NetworkPolicy | A Worker 使用 Calico 與 Go 網路 probe，包含同租戶正向控制及跨租戶實際封包拒絕 |
| HTML 主流程 | 真實伺服器表單的部署／good／replicas drift／broken／確認回滾／HTTP／確認刪除及 GC；使用頁面 CSRF／冪等鍵／期望版本 |
| 離線與恢復 | B 斷線服務延續、重連、Hub 原 DB／session 恢復、無 Hub 時 Operator 自主修復 |
| Hub 重建 | endpoint-only 更新，保留自訂 quota、receipt／ledger、Hub profile 與 session |

本機與對應 Hosted CI 的正式 M4 E2E 均有 PASS 證據，包含 owned cleanup。整合日誌為 `artifacts/release-validation/m3-m5.log`、`oidc-entry.log`；完整資源與 HTML 在 `artifacts/m4-e2e/<run>`。

真實瀏覽器也確認 Dex 登入、雙 Worker CONNECTED 與 HTML 登出。受保護頁面使用 same-origin Referrer-Policy，使 navigation POST 保有合法 Origin；OAuth 公開回應維持 no-referrer，缺失／null／外站 Origin 與 CSRF 拒絕規則保留。畫面證據在 `artifacts/m5-development/m4-browser-authenticated.png`、`m4-browser-logout.png`。

## 資源與限制

Hub 不掛 Worker kubeconfig、Worker 私鑰、CA signing key、Docker socket 或 workspace。帳號、subjects、DB 與憑證由 Git 忽略，正式 down 保留私有 state，只清理 ownership 相符的資源。

只有 Calico A Worker 宣稱 NetworkPolicy enforcement；B 不作相同保證。本機 loopback HTTP 是 LocalDemo 例外，正常部署要求 HTTPS 與 Secure cookie。leaf 24 小時、CA 48 小時，重新 up／重建 Hub 不續期；原地憑證輪替、node／主機重啟與備份恢復未驗收。`test-oidc` 的約 15 分鐘 JWT 自然到期通過，不代表已等待開發憑證 24 小時到期。
