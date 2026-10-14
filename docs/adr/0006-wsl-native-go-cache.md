# ADR 0006 — WSL 本機編譯快取

- 日期：2026-10-11
- 狀態：採納為本機開發選項；實際驗證另記錄。

M1 首次 Kubernetes 相依編譯與連結在 Windows D: 路徑耗時數分鐘，link 程序 CPU 約 2%、WSL I/O full 約 20%。後續驗收使用專案專用 `/tmp/hugofleet-go-cache.*`，由 `make cache-native` 複製已校驗 module archives，Go 在 Linux filesystem 解壓與編譯；原始碼、二進位交付與驗收日誌留在原專案。

`.local/native-cache.path` 記錄路徑，目錄的 `.hugofleet-root` 必須吻合此 checkout，且限制為上述 /tmp prefix。Go SDK 也由原本官方 SHA-256 校驗的 tar.gz 解壓到該目錄，避免跨 filesystem 載入標準庫。快取消失時回退 `.cache`；不修改全域 Go、WSL 或 Docker 設定，不遷移／刪除原快取，不影響 Kubernetes 資料。Docker Race 將同一快取掛載至 /cache、原始碼唯讀掛載；正式 image build 不包含任何快取。

## 2026-10-12：使用 Linux 磁碟及專用編譯暫存

新 checkout 的 M4 實測發現 WSL `/tmp` 為 7.8 GiB tmpfs，原有其它暫存加上新專案快取後，編譯 SQLite 時出現 `no space left on device`。新建快取改用 `/var/tmp/hugofleet-go-cache.*`；common.sh 接受新 prefix 與舊 `/tmp` prefix，仍必須核對 `.hugofleet-root`，並將 `GOTMPDIR` 設到同一專案快取的 `go-tmp`，避免大編譯使用共用 RAM tmpfs。

本輪核對精確路徑、專案登記、ownership 與 SDK 二進位後，將此次新建的專案快取移到 Linux 磁碟。Go module 目錄的唯讀模式曾使跨 filesystem move 的來源移除失敗；核對目的內容後，只清理該專案可再生來源殘餘。原 `.cache`、其它 `/tmp` 資料、舊 checkout 快取、DB／憑證與 WSL／Docker 全域設定保持不變。後續編譯與 Docker 快取掛載需分別實測，結果寫入 M4 驗收紀錄。

這是編譯環境選項，不改變應用行為或驗收 assertions；測試仍使用同一份 workspace 原始碼。WSL Linux filesystem 的使用依據見 [Microsoft 檔案儲存與效能說明](https://learn.microsoft.com/en-us/windows/wsl/filesystems#file-storage-and-performance-across-file-systems)。
