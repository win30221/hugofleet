# M4 驗收範圍

M4 在 Worker 主動 mTLS 架構上驗證真實 Dex OIDC、server-side session、DB membership、租戶 namespaces／RBAC／Quota／LimitRange／NetworkPolicy 與稽核。操作見 [M4 教學](../stages/m4/README.md)，設計見 [ADR 0010](adr/0010-oidc-tenants-and-worker-bootstrap.md)，跨階段實測與 CI 證據見 [驗證總覽](validation.md)。

## 如何使用本頁

第一次操作請先讀本階段教學；本頁用來選擇自動驗證與核對既有證據。只修改文件時執行 `make docs-check`，毋須啟動本頁整套測試。

| 命令 | 目的 |
|---|---|
| `make m4-build m4-test` | 編譯與 M4 單元測試 |
| `make proto-check verify-generated test-integration` | 協定、生成檔與共用 envtest |
| `make test-oidc` | 獨立真實 Dex 負向與約 15 分鐘自然到期 |
| `make test-e2e-m4` | 自行建置，驗證角色／Admission／Calico／GC 與清理 |

下方依序列出本階段需要的工具與驗收命令；已安裝的固定工具可沿用，不需準備其他階段的工具。一次執行一組 E2E，先依[停止流程](../stages/m4/README.md#停止與下次繼續)結束會占用相同連接埠的教學環境。

## 驗收命令

在專案根目錄的 Ubuntu WSL2／Linux Bash，以非 root 使用者執行。先釋放教學要求的 Hub／Dex ports：

```bash
make tools operator-tools envtest proto-tools
make m4-build m4-test
make proto-check verify-generated test-integration
make test-oidc
make test-e2e-m4
```

## 如何判斷本次執行結果

等待整個命令結束，核對測試與 owned cleanup 均成功且最終退出碼為 0。中途出現 PASS 不表示整體完成；失敗先保留輸出與 state，依[本階段排查](../stages/m4/README.md#操作遇到問題時)定位問題。下列已確認行為來自既有驗收，對應證據與程式提交見驗證總覽，不會自動套用到新修改。

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

## 查完後返回

本頁的測試不是初次學習的必經步驟。返回 [M4 整課驗證](../stages/m4/README.md#整課驗證與常見問題)核對手動操作；要停止或進下一課，前往 [M4 完成後的選擇](../stages/m4/README.md#完成這一課後去哪裡)。修改其他元件時，依[驗證總覽](validation.md#執行驗證)選擇測試。
