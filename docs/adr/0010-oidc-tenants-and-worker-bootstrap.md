# ADR 0010：OIDC、租戶授權與 Worker bootstrap

狀態：已採納。驗收範圍見 [M4 驗收](../m4-validation.md)。

## 問題

M3 的隨機管理 Token 只有本機單管理者防護，不能辨識使用者或隔離團隊。Hub 又不能取得 Worker cluster-admin，普通 Agent 也不能為了建立租戶而增加 Namespace／RBAC 寫入權限。

## 決策

M4 提供獨立的 OIDC Hub 入口，沿用 M2 的交易／版本／Operation runner 與 M3 的雙向 mTLS 通道。M2／M3 開發模式拒絕開啟已啟用安全模式的 DB。租戶 slug 為不可變 DNS label，namespace 固定為 hf-<slug>；禁止從請求接受任意 namespace 或 PodSpec。

身分以精確 issuer + sub 綁定；email 僅作顯示。Platform Admin 由本機受信任設定指定精確 subject，Owner／Developer／Viewer 由資料庫 membership 決定，忽略 JWT groups／role 及用戶自填角色。所有管理 API、HTML 頁面與 Operation／audit 查詢在 server-side 授權；變更的交易在冪等查詢前重新查驗角色，避免撤權後重送仍取得舊結果。

Authorization Code + S256 PKCE 使用伺服器保存的五分鐘登入交易：一次性 state、nonce、verifier 與瀏覽器 binding。先消耗交易再交換 code；ID Token 驗證 issuer、audience、JWKS signature、expiry、nonce。session 使用不透明隨機值，DB 只保存其 hash；過期時間不超過 ID Token，登出立即撤銷 session。登入 binding cookie 為 SameSite=Lax，平台 session 為 HttpOnly、SameSite=Strict；HTTPS 設 Secure，所有 session 認證變更驗證 CSRF 與精確 Origin。

HTTP 僅允許明確的 loopback Demo profile：主機 Publish 127.0.0.1、精確 Host／private Docker gateway，以及固定的 loopback Dex issuer。管理來源 IP 必須為 private／loopback；容器內 Dex 連線可將該 issuer 的 TCP destination 映射至同網路 private Dex 位址。此 Demo backchannel 明確使用 HTTP，仍核對 issuer／Host／JWT；正常模式要求 HTTPS 並禁止 Demo 網路 overrides。M4 不提供無驗證 Dev Auth。

Discovery 的 authorization／token／JWKS endpoint 與 backchannel wire URL 都要求同 issuer origin／path 且保持 canonical；拒絕 dot segments、重複斜線、需要 escaping 的路徑、query／fragment／userinfo。除了 `RawPath`，還核對 `EscapedPath() == Path`，涵蓋標準 escaping 與雙重編碼。這是刻意收窄的 Provider 相容範圍，避免 server／proxy 正規化路徑的歧義。

Worker namespace／Quota／LimitRange／精確 Roles 與 NetworkPolicy 由獨立 Go bootstrap 工具建立；管理員在自己的終端明確指定 owned Worker 的 kubeconfig，該憑證不傳入 Hub。工具只接受租戶 slug、固定資源欄位與已知部署設定，無任意 shell／YAML 功能。租戶建立分為管理員 bootstrap 與 Hub activation：Agent 以唯讀權限核對受保護的 bootstrap receipt、實際 ResourceQuota／LimitRange，再經 mTLS 回傳型別化 profile，Hub 才允許 Admin 啟用租戶／指定 Owner。未 provision、未授權或離線 Worker 不能啟用，UI 不將 DB row 當成 Kubernetes 建立成功。

普通 Agent 的 trusted allowlist 與 namespaced RoleBinding 決定可操作的租戶；只能寫 Application CR、讀所需 workload/status 與自己的 ledger。bootstrap receipt 不可由 Agent／Operator／Sample 修改。每 namespace 的 Operator 維持 namespaced Role；Sample Pod 不掛 ServiceAccount token。ledger 綁定 tenant、名稱與 UID，避免同 app_id 跨 namespace 重用。

ResourceQuota 包含 requests.cpu／requests.memory／limits.cpu／limits.memory／pods，LimitRange 設預設與每容器界限。Hub 的 replicas + surge 估算只作提前拒絕與提示；實際 Pod Admission 與 FailedCreate／ReplicaFailure 才是權威。Security Profile 使用 enforcing CNI，驗證同租戶流量成功、跨租戶封包被阻擋，不能僅以 Policy object 存在判 PASS。

## 固定版本與來源

- Dex 2.46.0：真實 password database／bcrypt、Authorization Code 與強制 S256 PKCE；靜態帳號僅限本機 Demo。[官方設定](https://raw.githubusercontent.com/dexidp/dex/v2.46.0/config.yaml.dist)、[release](https://github.com/dexidp/dex/releases/tag/v2.46.0)。
- coreos/go-oidc/v3 3.21.0、go-jose/v4 4.1.5：Go 1.27.2 符合其最低版要求；jose 4.1.5 包含 JWT／JWK 安全修正。[OIDC go.mod](https://raw.githubusercontent.com/coreos/go-oidc/v3.21.0/go.mod)、[jose release](https://github.com/go-jose/go-jose/releases/tag/v4.1.5)。
- golang.org/x/crypto 0.57.0：Go bcrypt 用於產生私有 Demo 設定，與 Dex 2.46 的官方相依一致。[go.mod](https://raw.githubusercontent.com/golang/crypto/v0.57.0/go.mod)。
- Calico 3.33.0：官方列出 Kubernetes 1.35／1.36／1.37；Security Profile 使用 kind disableDefaultCNI 與指定 pod subnet。[相容性](https://docs.tigera.io/calico/latest/getting-started/kubernetes/requirements)、[kind 安裝](https://docs.tigera.io/calico/latest/getting-started/kubernetes/kind)。

## 限制與取捨

Namespace 與安全政策的管理權留在 Worker 管理員，不把 cluster-admin 包成遠端管理 API。平台 Admin 可啟用已 bootstrap 的 profile、管理成員與觀察配額；改變 namespace 安全政策／quota 須以受控 bootstrap 工具更新再同步，不能只改 Hub DB。正式 issuer／client／subject 設定由部署管理員供應；本機 Dex 測試密碼／憑證不代表企業 IAM。仍為單 Hub／SQLite，不提供 HA。M5 的觀測與交付另作 ADR。

## 閱讀後返回

本頁用來查設計原因，不是安裝步驟。返回 [M4 原理與進階閱讀](../../stages/m4/README.md#原理與進階閱讀)，或從[選課與回訪](../../stages/README.md#回來繼續哪一課)接續使用此共用設計的其他階段。其他設計問題見 [ADR 導覽](../README.md#設計問題與-adr-對照)。
