# ADR 0009：依教學階段分類原始碼

日期：2026-10-11。使用者要求每個階段的程式可找到、教學資源隔離，並保留前面階段的獨立操作。

將 Go 程式、API 型別、YAML／JSON、Bash、Dockerfile 與 Go 測試實際集中在 stages/m0～m3，依最初引入的階段分類。使用者進一步要求所有教學資源集合在同一階段資料夾，因此完整教學與程式導讀合併為該資料夾的 README.md；根目錄放 run.sh 與範例，scripts/ 放該階段的工具／控制／驗收腳本。README 提供新增功能、入口、閱讀順序、測試與共用修改對照。.local/tutorials 保留私有執行資料，docs 保留契約與驗收證據。跨階段共用的 Docker／kind／固定版本工具保留根目錄 scripts／tools／deploy。

整併前建立受影響的 135 份原始檔案快照，逐一 SHA-256 比對；取代的舊教學與重複資源另存 archived。使用者隨後授權清理，這份備份已歸位至 .local/archive/backups/stage-collection-backup-20261011。另於本次清理前保存 16 份將修改的來源／文件至 .local/archive/backups/cleanup-20261011/before，複製時核對 SHA-256。備份被 Git 忽略，保留還原依據。

仍使用單一 module github.com/win30221/hugofleet。M0 的應用被後面各階段沿用；M1 API／controller 被 M2／M3 共用，M2 Hub 被 M3 擴充。跨階段 Go 套件不使用會限制 importer 的 internal 路徑；pricing-api 私有的 internal/pricing 保留。這是目前程式依來源階段的分類，不建立四份歷史快照或複製共用邏輯。M3 導讀明列修改的 M2 Hub 與 schema 003；M2 舊入口仍預設本機模式。

同步修改 import、proto go_package／生成位置、controller-gen、Docker COPY／build、Make、E2E 入口、envtest CRD 路徑與所有文件連結；API wire／SQLite schema／RBAC／runtime 路徑契約維持原意。新增各階段 build／Unit 命令方便閱讀驗證；真實叢集仍由各階段獨立入口與 kind E2E 驗收。

搬移後的驗證應針對受影響的檔案連結、import、腳本與 Docker 建置路徑；有功能變更或具體失敗時才擴大到相關功能測試。先前實測範圍與結果以 [M3 驗收紀錄](../m3-validation.md) 為準。使用者已要求先確認資料夾與文件的整理結果，再恢復驗證；目前不執行新的檢查、Demo 或叢集重建。此整理只涵蓋已授權的 M0～M3，M4／M5 尚未開始。

後續清理將 M0～M3 的階段操作實作各自歸入 scripts/control.sh；共用 kind lifecycle 移至根 scripts/kind.sh，hf.sh 保留命令分派／相容入口與跨階段開發命令。既有 Make target 名稱保留。根 README 精簡為總覽／環境準備，詳細 M2 操作與舊開發入口回到 M2，pricing-api 契約回到 M0；docs/stages.md 合併至 stages/README.md，原文件與來源備份存入上述 snapshot。

臨時工具、草稿、舊 state 依用途歸檔至 .local/archive；原 artifacts 證據維持原路徑。移除可再生的舊 Windows Go 快取、重複 gomod 快取、Python bytecode、未完成下載、一次性編譯工具及空 state，不刪除正式來源／測試／DB／憑證。此輪只完成檔案整理與清理，沒有重新驗證操作腳本或程式。
