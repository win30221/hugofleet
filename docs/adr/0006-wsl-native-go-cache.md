# ADR 0006 — WSL 本機編譯快取

- 狀態：已採納，作為本機開發選項。

## 背景

Windows 掛載上的 Go 套件掃描、解壓與連結容易受跨 filesystem I/O 影響。大型 Kubernetes／SQLite 編譯也需要足夠暫存空間；WSL 的 `/tmp` 可能是容量較小的 RAM tmpfs。

## 決策

`make cache-native` 在 Linux 磁碟建立專案專用 `/var/tmp/hugofleet-go-cache.*`，複製已校驗 module archives，並從官方 SHA-256 驗證過的 tar.gz 解壓固定 Go SDK。原始碼、bin 與 artifacts 仍留在 checkout。

`.local/native-cache.path` 登記快取路徑；`common.sh` 必須核對 prefix 與 `.hugofleet-root` 所記錄的 checkout。設定 GOCACHE、GOMODCACHE 及快取內 `go-tmp` 的 GOTMPDIR，避免大型編譯耗盡共用 RAM tmpfs。相容舊 `/tmp/hugofleet-go-cache.*` prefix，但新快取使用 `/var/tmp`。

Docker Race 使用相同快取掛載至 `/cache`，來源唯讀，正式 image 不含快取。快取消失時回退專案 `.cache`；不修改全域 Go、WSL 或 Docker 設定。

## 限制

此選項只影響編譯環境，不改變應用與驗收條件。快取屬於單一 checkout，不能借用其他專案的登記，也不能以清理共用 `/tmp` 取代空間診斷。需預留 Linux 磁碟空間；DB、憑證與 Kubernetes state 不屬於可再生的 Go 快取。

官方依據：[Microsoft 檔案儲存與效能說明](https://learn.microsoft.com/en-us/windows/wsl/filesystems#file-storage-and-performance-across-file-systems)。
