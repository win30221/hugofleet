# M0～M5 互動操作動畫

[開啟動畫教室](index.html)。每章都是可直接用 Chrome／Edge 開啟的離線 HTML，不需要啟動 Hub、Docker、kind 或本機伺服器。

動畫用具名元件、控制方向、Pod 身分、版本履歷與逐段解說呈現因果。畫面是教學快照，不是目前叢集觀測，也不是 kind 驗收證據。機制與實作來源見 [分鏡核對](storyboard.md)。

## 各章入口

| 章節 | 內容 | 步驟／分鏡 | 播放時間，不含停頓 | 實作 |
|---|---|---|---|---|
| [M0](m0.html) | API 用途、映像、部署、探針、換版、擴容、恢復 | 12／43 | 5:58 | [教學](../../stages/m0/README.md) |
| [M1](m1.html) | CRD、Application、Operator、漂移、故障、GC | 12／41 | 4:47 | [教學](../../stages/m1/README.md) |
| [M2](m2.html) | 表單、SQLite、202／Applied／Ready、衝突、快照回滾 | 11／36 | 4:12 | [教學](../../stages/m2/README.md) |
| [M3](m3.html) | Agent 主動建連、A／B 部署、離線自主修復、重連 | 10／35 | 4:05 | [教學](../../stages/m3/README.md) |
| [M4](m4.html) | OIDC、租戶 proof、角色、跨租戶拒絕、配額、封包 | 10／22 | 2:34 | [教學](../../stages/m4/README.md) |
| [M5](m5.html) | Dashboard、故障追查、回滾、metrics／trace、Helm | 12／42 | 4:54 | [教學](../../stages/m5/README.md) |

共 67 步、219 段分鏡；M1～M5 為 55 步、176 段分鏡。等待時間經壓縮，秒數不代表真實叢集的排程、探針或終止耗時。

## 播放方式

- 點「播放這一步」開始，預設每一步結束停住；確認看懂後按「下一步」。
- 勾選「連續播放本章」可自動接續，本章結束仍停止，不自動跳到下一章。
- 可以暫停、重播、拖曳本步時間軸、調整速度，或直接點分鏡編號與左側步驟。
- 頁面取得焦點時，Space 播放／暫停，左右方向鍵切換步驟；原生輸入與按鈕保留自己的鍵盤行為。
- 系統「減少動態」只減少位移與轉場效果，**不會停用播放／重播**。可在頁面切換完整動態效果。
- 深淺色可切換。切到背景分頁時暫停動畫計時，返回後續播。
- 章節各有 `m0.html`～`m5.html`；步驟網址可使用 `#step-1` 等片段。舊的 `index.html#m0`～`index.html#m5` 會轉到對應新頁。

命令摺疊在畫面下方，僅供對照。頁面不執行 shell、不連線 Hub、不建立或刪除 Kubernetes 資源。M0 的清理按鈕也只切換示意。

## 畫面語意

每個 Pod 保留自己的 ID、映像與 revision。新版以額外 Pod 出現，通過 readiness 後才接替舊版；退出中的 Pod 顯示 Terminating。一般 Service 的可用端點只列 Ready Pod。跳回其他分鏡會恢復該快照，已不存在的 Pod 不應殘留。

M1 的漂移先呈現 CR 與 Deployment spec 不一致，Operator 修回 spec；不預設一定有 Pod 消失。M2／M5 依教學主線選 rev 2 快照建立 rev 4，恢復 v2-good／兩副本，保留 rev 3 FAILED。M3 同時展示 Hub 的觀測與當地工作負載，UNKNOWN 不代表 Pod 停止。

M4 配額是明示的獨立情境：兩個既有健康 Pod、surge 建立被 Admission 拒絕，沒有第三個壞 Pod。NetworkPolicy 需要實際封包測試，動畫不代替 A Worker 的 CNI 驗收。M5 的 B 通道演練保留「尚無業務應用」，不把 Agent 連線當成 B 業務服務已驗證。

## 來源與建置

| 檔案 | 用途 |
|---|---|
| [build.mjs](build.mjs) | 建置全部離線頁面與六章入口 |
| [m0-story.mjs](m0-story.mjs) | M0 分鏡資料與 Pod 生命週期 |
| [m0-page.html](m0-page.html)／[m0-player.js](m0-player.js)／[m0.css](m0.css) | M0 模板、播放控制與共用視覺樣式 |
| [build-m0.mjs](build-m0.mjs) | M0 建置與資料檢查 |
| [course-story.mjs](course-story.mjs) | M1～M5 分鏡、發布履歷與狀態 |
| [course-page.html](course-page.html)／[course-player.js](course-player.js)／[course.css](course.css) | M1～M5 共用架構圖、Pod 顯示與播放器 |
| [build-courses.mjs](build-courses.mjs) | M1～M5 建置與機制檢查 |
| [storyboard.md](storyboard.md) | 機制與實作來源核對 |

播放不需要 Node。修改後執行：

```bash
node docs/animations/build.mjs
node docs/animations/build.mjs --check
node --check docs/animations/m0-player.js
node --check docs/animations/course-player.js
make docs-check
```

建置檢查包含來源／HTML 一致性、Pod 身分不原地換版、位置不重疊、壞版保留兩個健康副本、配額拒絕不建立 Pod，以及回滾沿用來源配置並保留失敗履歷。瀏覽器另檢查播放、暫停、重播、步末停止、連播、跳步、版面及減少動態模式。這些檢查不等於真實 Kubernetes 驗收。

原工作台的 [Anatomy kit](vendor/anatomy/iso-kit.mjs) 與 [MIT License](vendor/anatomy/LICENSE) 保留，來源固定於 Anatomy 提交 `e9488296a44211c42701ff71705b7901470ec78f`。原始工作台來源檔保留供參考，不再作為播放入口。新版採直接標示實際元件的平面架構分鏡，共用同一份狀態驅動畫面與解說；沒有外部字型、CDN 或 WebGL 依賴。

## 真實素材與驗證邊界

[既有 M5 操作剪輯](../media/m5-demo.html) 使用真實瀏覽器截圖；[既有回滾 trace](../media/m5-rollback-trace.html) 使用已收集 spans。兩者不是此次動畫修改的新驗收。實作與測試命令、證據及限制見 [驗證總覽](../validation.md)。

本資料夾交付互動 HTML，沒有新增 MP4 或配音，也不包含帳密、session、憑證或 kubeconfig。
