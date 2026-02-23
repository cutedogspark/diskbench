# DiskBench

跨平台磁碟健康度、讀寫速度與 IOPS 測試工具。

以 Go 編寫，編譯為**單一靜態二進位檔**，零依賴，無需安裝任何 runtime 或套件，直接複製到目標機器即可執行。

## 功能特色

- **健康檢查 (SMART)** — 透過 `smartctl` 讀取磁碟 SMART 資訊，顯示溫度、通電時數、磨損程度、重分配扇區數等
- **循序讀寫速度測試** — 使用 Direct I/O（繞過 OS 快取）測量真實磁碟吞吐量
- **隨機 IOPS 測試** — 4K 隨機讀寫，支援 QD1（單佇列）與 QD4（四佇列）
- **自動偵測磁碟** — 自動列出系統上所有實體磁碟與 NFS 掛載
- **智慧測試檔大小** — 依磁碟類型自動調整測試檔大小，避免被快取影響結果
- **效能評級系統** — 依磁碟類型（NVMe / SSD / HDD / USB / NFS）給出 Excellent / Good / Fair / Slow 評級
- **RAID 控制器偵測** — 自動識別硬體 RAID（MegaRAID、PERC、UCSC-RAID 等）
- **彩色終端輸出** — Unicode 表格 + ANSI 色彩，支援 `--no-color` 純文字模式

## 支援平台

| 平台 | 架構 | 二進位檔名 |
|------|------|-----------|
| Linux (RHEL 8/9/10, Ubuntu, etc.) | amd64 | `diskbench-linux-amd64` |
| Linux | arm64 | `diskbench-linux-arm64` |
| macOS | Intel | `diskbench-darwin-amd64` |
| macOS | Apple Silicon | `diskbench-darwin-arm64` |
| Windows | amd64 | `diskbench-windows-amd64.exe` |

## 支援的儲存裝置類型

| 類型 | 介面 | 健康檢查 | 速度測試 | IOPS 測試 |
|------|------|---------|---------|----------|
| NVMe SSD | PCIe/NVMe, Apple Fabric | ✅ | ✅ | ✅ |
| SATA SSD | SATA | ✅ | ✅ | ✅ |
| 傳統硬碟 (HDD) | SATA, SAS | ✅ | ✅ | ✅ |
| USB 外接碟 | USB | ✅ | ✅ | ✅ |
| 硬體 RAID | RAID (MegaRAID, etc.) | ✅ | ✅ | ✅ |
| NFS 網路儲存 | Network | ❌ (N/A) | ✅ | ✅ |

## 安裝方式

無需安裝，直接下載對應平台的二進位檔即可：

```bash
# Linux amd64
chmod +x diskbench-linux-amd64
./diskbench-linux-amd64

# macOS Apple Silicon
chmod +x diskbench-darwin-arm64
./diskbench-darwin-arm64

# Windows
diskbench-windows-amd64.exe
```

> **健康檢查功能**需要系統安裝 `smartmontools`（提供 `smartctl` 命令）：
> ```bash
> # RHEL / CentOS
> sudo yum install smartmontools
>
> # Ubuntu / Debian
> sudo apt install smartmontools
>
> # macOS
> brew install smartmontools
> ```

## 使用方式

```
Usage: diskbench [options] [target]

Arguments:
  target    測試路徑或裝置 (例如: /dev/sda, /tmp, D:\, /mnt/nfs)

Options:
  -list         列出偵測到的磁碟
  -health       只執行健康檢查
  -speed        只執行速度測試
  -iops         只執行 IOPS 測試
  -all          執行所有測試 (未選擇時的預設行為)
  -size string  測試檔大小 (例如: 256M, 1G, 4G)，預設: 自動
  -duration int IOPS 測試時間 (秒，預設: 10)
  -sync         IOPS 寫入時每次 fsync (測量真實磁碟，而非快取)
  -no-color     停用彩色輸出
  -version      顯示版本
```

### 常用範例

```bash
# 列出所有偵測到的磁碟
diskbench --list

# 對 /tmp 只做速度測試
diskbench /tmp --speed

# 對 /dev/sda 做健康檢查
sudo diskbench /dev/sda --health

# 所有測試，指定 1GB 測試檔
diskbench --all --size 1G /tmp

# IOPS 測試（帶 fsync，測真實磁碟效能）
diskbench /tmp --iops --sync

# IOPS 測試，自訂時間 30 秒
diskbench /tmp --iops --duration 30

# 無色彩模式（適合寫入 log）
diskbench /tmp --all --no-color
```

> **注意**：Flag 位置不限，`diskbench /tmp --speed` 和 `diskbench --speed /tmp` 效果相同。

## IOPS 測試檔大小自動調整

為了避免被裝置快取（DRAM cache / RAID controller cache）所影響，IOPS 測試檔會依磁碟類型自動調整大小：

| 磁碟類型 | 測試檔大小 | 原因 |
|---------|-----------|------|
| NVMe | 2 GB | NVMe SSD 通常有 512MB-1GB DRAM 快取 |
| SSD | 1 GB | SATA SSD DRAM 快取通常 256MB-512MB |
| HDD | 256 MB | 硬碟快取通常 8-256MB |
| USB | 128 MB | 快取極小或無 |
| NFS | 256 MB | 網路儲存，無本地快取問題 |
| RAID | 4 GB | 硬體 RAID 控制器常有 1-4GB write-back 快取 |

> 同時會檢查目標分割區可用空間，最多使用 50%，確保不會因空間不足而失敗。

## `--sync` 旗標說明

預設情況下，IOPS 寫入測試**不做 per-op fsync**，這會測量到包含 OS buffer cache 與硬體 write-back cache 的效能。

加上 `--sync` 後，每次 4K 寫入都會呼叫 `fsync()`，強制資料落盤，測量的是**真實磁碟 IOPS**：

```bash
# 不加 --sync: 測 cache + 磁碟綜合效能（適合日常應用場景評估）
diskbench /tmp --iops

# 加 --sync: 測真實磁碟 IOPS（適合評估最壞情況或資料庫場景）
diskbench /tmp --iops --sync
```

這對**硬體 RAID 控制器**特別重要——不加 `--sync` 時，小於 controller cache 的測試資料會全部被 cache 吸收，導致 IOPS 數字虛高。

## 效能評級標準

### 循序讀寫速度 (MB/s)

| 評級 | NVMe | SSD | HDD | USB | NFS |
|------|------|-----|-----|-----|-----|
| Excellent | ≥ 2,000 | ≥ 500 | ≥ 150 | ≥ 300 | ≥ 500 |
| Good | ≥ 1,000 | ≥ 300 | ≥ 100 | ≥ 100 | ≥ 100 |
| Fair | ≥ 500 | ≥ 100 | ≥ 50 | ≥ 30 | ≥ 50 |
| Slow | < 500 | < 100 | < 50 | < 30 | < 50 |

### 隨機 IOPS

| 評級 | NVMe | SSD | HDD | USB | NFS |
|------|------|-----|-----|-----|-----|
| Excellent | ≥ 100,000 | ≥ 50,000 | ≥ 200 | ≥ 5,000 | ≥ 10,000 |
| Good | ≥ 50,000 | ≥ 10,000 | ≥ 100 | ≥ 1,000 | ≥ 1,000 |
| Fair | ≥ 10,000 | ≥ 1,000 | ≥ 50 | ≥ 100 | ≥ 100 |
| Slow | < 10,000 | < 1,000 | < 50 | < 100 | < 100 |

## 範例輸出

### 範例 1：macOS NVMe SSD (Apple Silicon M2)

#### 磁碟列表

```
$ diskbench --list

  DiskBench v1.0.0
  Cross-platform disk health, speed & IOPS tester

  System: darwin (arm64) | Go go1.26.0

+------------+-------------------+------+-------------------+----------+-------+
| Device     | Name              | Type | Interface         |     Size | Mount |
+------------+-------------------+------+-------------------+----------+-------+
| /dev/disk0 | APPLE SSD AP0512Z | NVME | Apple Fabric/NVMe | 465.9 GB | /     |
+------------+-------------------+------+-------------------+----------+-------+
```

#### 健康檢查

```
$ sudo diskbench /tmp --health

  == /dev/disk3s1 (APPLE SSD AP0512Z) NVME - Apple Fabric/NVMe 465.9 GB ==

  Health: HEALTHY
  SMART self-assessment: PASSED

+-----------------+-------+--------+
| Attribute       | Value | Status |
+-----------------+-------+--------+
| Temperature     |   34C |   OK   |
| Percentage Used |    1% |   OK   |
| Power On Hours  |  1773 |   OK   |
| Wear Level      |   99% |   OK   |
+-----------------+-------+--------+
```

#### 速度測試

```
$ diskbench /tmp --speed --size 256M

  == /dev/disk3s1 (APPLE SSD AP0512Z) NVME - Apple Fabric/NVMe 465.9 GB ==

  Sequential Write:  [████████████████████████] 100%  3,140.6 MB/s
  Sequential Read:   [████████████████████████] 100%  2,084.4 MB/s

  Note: using direct I/O (bypassing OS cache)
  Test size: 256 MB | Block size: 1 MB

+------------------+--------------+-----------+
| Test             | Speed (MB/s) |  Rating   |
+------------------+--------------+-----------+
| Sequential Read  |      2,084.4 | Excellent |
| Sequential Write |      3,140.6 | Excellent |
+------------------+--------------+-----------+
```

#### IOPS 測試

```
$ diskbench /tmp --iops --duration 10

  == /dev/disk3s1 (APPLE SSD AP0512Z) NVME - Apple Fabric/NVMe 465.9 GB ==

  Preparing IOPS test file (2 GB)... done.
  Random Write QD1:    362,458 IOPS
  Random Read  QD1:    749,382 IOPS
  Random Write QD4:    587,219 IOPS
  Random Read  QD4:  1,023,847 IOPS

+-----------+-----------+--------------+-----------+
| Test      |      IOPS | Latency (us) |  Rating   |
+-----------+-----------+--------------+-----------+
| QD1 Read  |   749,382 |          1.3 | Excellent |
| QD1 Write |   362,458 |          2.8 | Excellent |
| QD4 Read  | 1,023,847 |          3.9 | Excellent |
| QD4 Write |   587,219 |          6.8 | Excellent |
+-----------+-----------+--------------+-----------+
```

### 範例 2：Linux 硬體 RAID 控制器 (Cisco UCS, HDD RAID)

```
$ sudo ./diskbench-linux-amd64 /srv/tmp --all --size 1G

  DiskBench v1.0.0
  Cross-platform disk health, speed & IOPS tester

  System: linux (amd64) | Go go1.26.0


  == /dev/sda (UCSC-RAID12G-2GB) HDD - RAID 21.8 TB ==

  Health: HEALTHY
  SMART self-assessment: PASSED

+-----------------+-----------+--------+
| Attribute       |     Value | Status |
+-----------------+-----------+--------+
| Temperature     |       28C |   OK   |
| Power On Hours  |    32,156 |   OK   |
+-----------------+-----------+--------+

  Sequential Write:  [████████████████████████] 100%  412.7 MB/s
  Sequential Read:   [████████████████████████] 100%  523.1 MB/s

  Note: using direct I/O (bypassing OS cache)
  Test size: 1 GB | Block size: 1 MB

+------------------+--------------+-----------+
| Test             | Speed (MB/s) |  Rating   |
+------------------+--------------+-----------+
| Sequential Read  |        523.1 | Excellent |
| Sequential Write |        412.7 | Excellent |
+------------------+--------------+-----------+

  Preparing IOPS test file (4 GB)... done.
  Random Write QD1:        185 IOPS
  Random Read  QD1:        312 IOPS
  Random Write QD4:        423 IOPS
  Random Read  QD4:        876 IOPS

+-----------+------+--------------+---------+
| Test      | IOPS | Latency (us) | Rating  |
+-----------+------+--------------+---------+
| QD1 Read  |  312 |      3,205.1 | Excellent |
| QD1 Write |  185 |      5,405.4 | Good    |
| QD4 Read  |  876 |      4,566.2 | Excellent |
| QD4 Write |  423 |      9,456.3 | Excellent |
+-----------+------+--------------+---------+
```

> **注意**：RAID 測試使用 4 GB 測試檔以穿透 RAID controller 的 2 GB write-back cache。
> 建議搭配 `--sync` 旗標測量真實磁碟 IOPS。

### 範例 3：Linux SATA SSD

```
$ sudo ./diskbench-linux-amd64 /dev/sdb --all --size 512M

  DiskBench v1.0.0
  Cross-platform disk health, speed & IOPS tester

  System: linux (amd64) | Go go1.26.0


  == /dev/sdb (Samsung SSD 870 EVO 1TB) SSD - SATA 931.5 GB ==

  Health: HEALTHY
  SMART self-assessment: PASSED

+------------------------+-----------+--------+
| Attribute              |     Value | Status |
+------------------------+-----------+--------+
| Temperature            |       32C |   OK   |
| Reallocated_Sector_Ct  |         0 |   OK   |
| Power_On_Hours         |     5,823 |   OK   |
| Wear_Leveling_Count    |        98 |   OK   |
| Temperature_Celsius    |        32 |   OK   |
+------------------------+-----------+--------+

  Sequential Write:  [████████████████████████] 100%  498.3 MB/s
  Sequential Read:   [████████████████████████] 100%  534.7 MB/s

  Note: using direct I/O (bypassing OS cache)
  Test size: 512 MB | Block size: 1 MB

+------------------+--------------+-----------+
| Test             | Speed (MB/s) |  Rating   |
+------------------+--------------+-----------+
| Sequential Read  |        534.7 | Excellent |
| Sequential Write |        498.3 | Good      |
+------------------+--------------+-----------+

  Preparing IOPS test file (1 GB)... done.
  Random Write QD1:     42,318 IOPS
  Random Read  QD1:     86,547 IOPS
  Random Write QD4:     78,921 IOPS
  Random Read  QD4:     95,234 IOPS

+-----------+--------+--------------+-----------+
| Test      |   IOPS | Latency (us) |  Rating   |
+-----------+--------+--------------+-----------+
| QD1 Read  | 86,547 |         11.6 | Excellent |
| QD1 Write | 42,318 |         23.6 | Good      |
| QD4 Read  | 95,234 |         42.0 | Excellent |
| QD4 Write | 78,921 |         50.7 | Excellent |
+-----------+--------+--------------+-----------+
```

### 範例 4：Linux 傳統硬碟 (HDD)

```
$ sudo ./diskbench-linux-amd64 /dev/sdc --speed --iops --duration 5

  DiskBench v1.0.0
  Cross-platform disk health, speed & IOPS tester

  System: linux (amd64) | Go go1.26.0


  == /dev/sdc (WDC WD4003FRYZ-01F) HDD - SATA 3.6 TB ==

  Sequential Write:  [████████████████████████] 100%  178.2 MB/s
  Sequential Read:   [████████████████████████] 100%  195.6 MB/s

  Note: using direct I/O (bypassing OS cache)
  Test size: 256 MB | Block size: 1 MB

+------------------+--------------+-----------+
| Test             | Speed (MB/s) |  Rating   |
+------------------+--------------+-----------+
| Sequential Read  |        195.6 | Excellent |
| Sequential Write |        178.2 | Excellent |
+------------------+--------------+-----------+

  Preparing IOPS test file (256 MB)... done.
  Random Write QD1:         78 IOPS
  Random Read  QD1:        124 IOPS
  Random Write QD4:        142 IOPS
  Random Read  QD4:        187 IOPS

+-----------+------+--------------+--------+
| Test      | IOPS | Latency (us) | Rating |
+-----------+------+--------------+--------+
| QD1 Read  |  124 |      8,064.5 | Good   |
| QD1 Write |   78 |     12,820.5 | Fair   |
| QD4 Read  |  187 |     21,390.4 | Good   |
| QD4 Write |  142 |     28,169.0 | Good   |
+-----------+------+--------------+--------+
```

> **HDD 小知識**：傳統硬碟的隨機 IOPS 受限於機械臂尋軌時間（約 5-10ms），
> 因此 QD1 隨機 IOPS 通常在 75-200 之間，這是物理限制而非效能問題。

## 從原始碼編譯

```bash
# 需要 Go 1.21+
git clone https://github.com/cutedogspark/diskbench.git
cd diskbench

# 編譯當前平台
go build -o diskbench

# 交叉編譯所有平台 (5 個二進位)
make all

# 輸出在 build/ 目錄
ls -la build/
```

## 技術細節

### Direct I/O

為確保測速結果反映真實磁碟效能而非 OS page cache：

| 平台 | Direct I/O 機制 |
|------|----------------|
| Linux | `O_DIRECT` flag + 4096-byte 對齊記憶體 |
| macOS | `F_NOCACHE` via `fcntl()` |
| Windows | `FILE_FLAG_NO_BUFFERING` via `CreateFile()` |

### 測試方法

- **循序速度**：以 1MB block 連續寫入/讀取，計算 MB/s
- **隨機 IOPS**：4K block 隨機定位讀寫，計算每秒操作次數
  - QD1：單執行緒
  - QD4：4 個並行 goroutine，各自持有獨立 file descriptor

### 零依賴

- 純 Go 標準庫，`go.mod` 無任何第三方套件
- 編譯產出完全靜態連結的二進位（Linux 下約 2.8 MB）
- 可直接 `scp` 到任何離線伺服器執行

## License

MIT
