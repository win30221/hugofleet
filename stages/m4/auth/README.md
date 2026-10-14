# OIDC 與平台 session

操作從 [M4 教學](../README.md) 開始。核心在 [oidc.go](oidc.go)，Unit 在 [oidc_test.go](oidc_test.go)，真實 Dex／Hub 驗收在 [test/oidc](../test/oidc/)。

`Begin` 產生一次性 state、nonce、S256 PKCE verifier 和 browser binding；私有登入交易保存於 SQLite，期限五分鐘。瀏覽器僅收到 nonce／challenge／state 與 HttpOnly binding cookie，verifier 不交給瀏覽器。

`Complete` 先消耗登入交易，再檢查 binding、交換 code、核對 JWT 簽章／issuer／audience／expiry／nonce。失敗不能重用該 state。受信任身分為精確 issuer + sub；email 可更新，groups／role 不授予權限。原始 ID／access Token 不寫入資料庫、日誌或 audit。

成功後建立隨機平台 session，DB 僅存 token hash，TTL 不超過 ID Token 到期與設定的 session TTL。session cookie 為 HttpOnly／SameSite=Strict，正常 HTTPS 設 Secure。每次管理請求向 DB 查 session 與當前角色，登出立即撤銷。Cookie 變更請求另核對精確 Origin 與 CSRF；ID／access Token／Bearer 不能作平台 session。

`Config.Validate` 的正常模式要求 HTTPS、confidential client secret 及有界 TTL。LocalDemo 僅支援 127.0.0.1 的 HTTP issuer／UI；Docker Hub 可透過受信任的單一 `IssuerDial` IP 對該 issuer 做 TCP 映射，仍核對精確 issuer／Host 與 JWT。`HTTPClientIP` 限精確 private／loopback IP，不能設為公開位址；它是 Docker 入口來源核對，不授予平台角色。正常 HTTPS 模式禁止這些 override。Discovery 與後續 HTTP 限同一 issuer origin／path、回覆大小、timeout，禁止任意 redirect 或 proxy。

目前 Provider 必須將 authorization／token／JWKS endpoint 放在 issuer 的同 origin／path 下，且 URL 路徑須保持 canonical：拒絕 dot segments、重複斜線及需要 escaping 的路徑，包括雙重編碼、反斜線與空白。核對 `EscapedPath() == Path`，不能只靠可能為空的 `RawPath`。這是明確限制 Provider 相容範圍，避免 proxy／server 的路徑正規化歧義。Provider 須宣告 S256 PKCE，並使用 RS256 或 ES256 簽章；已實測 Dex，不宣稱支援所有企業 IdP 的 endpoint 配置。

真實測試涵蓋完整 Code／PKCE、nonce／state／binding 拒絕、Dex verifier／redirect 拒絕、code replay、groups 不提升角色、Token 邊界、session 過期／登出。JWT 測試等待 Dex Token 真正到期，不以修改時鐘替代。Unit 的簽章 Provider fixture 只用於單元邊界，不列為 Dex E2E 證據。

已有同一 state 的 Dex／私有 subjects 時，可獨立重驗目前 Auth 程式的真實 backchannel；此測試使用自己的 SQLite，會停在 Dex 的 callback redirect，再由 Auth 交換 code，不呼叫正在運作的 Hub callback：

```bash
source scripts/common.sh
export HF_M4_STATE="$PWD/.local/tutorials/m4" # 改成正在運作的同一 state
go test -tags=oidcintegration -count=1 -timeout=2m \
  -run TestRealDexCanonicalBackchannel -v ./stages/m4/test/oidc
```
