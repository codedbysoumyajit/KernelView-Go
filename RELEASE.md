# 🚀 KernelView-Go v1.2.0 Release Notes

We are excited to release **KernelView-Go v1.2.0**! This release brings full-fledged **Android / Termux CPU & GPU hardware detection**, complete **terminal raw-mode crash protection and signal handling**, vendor-themed ASCII brand logos for mobile GPUs, and major **regex performance optimizations**.

---

## 🌟 What's New in v1.2.0

### 📱 1. Deep Android & Termux Hardware Scraper
- **SoC Model Identifier Resolver**: Scans Android system properties (`ro.soc.model`, `ro.soc.manufacturer`, `ro.board.platform`) and `/proc/cpuinfo` to resolve exact mobile processors:
  - **Qualcomm Snapdragon**: e.g. `Qualcomm Snapdragon 8 Gen 3 (SM8650)`, `Snapdragon 8 Gen 2`, `Snapdragon 888`, `Snapdragon 778G`, etc.
  - **Google Tensor**: e.g. `Google Tensor G4`, `Google Tensor G3`, `Google Tensor G2`.
  - **MediaTek Dimensity / Helio**: e.g. `MediaTek Dimensity 9300`, `Dimensity 8100`, `Helio G85`.
  - **Samsung Exynos**: e.g. `Samsung Exynos 2400`, `Exynos 2200`, `Exynos 990`.
- **ARM Max CPU Clock Frequency Aggregator**: Queries `/sys/devices/system/cpu/cpu*/cpufreq/cpuinfo_max_freq` across all CPU cores to accurately report maximum clock speeds (e.g. `3.00 GHz`).
- **Mobile GPU & Sysfs Telemetry**:
  - Direct sysfs nodes for Qualcomm Adreno (`/sys/class/kgsl/kgsl-3d0/gpu_model` & `gpubusy`) and ARM Mali (`/sys/class/misc/mali0/device/gpuinfo`).
  - Resolves mobile GPU models (Adreno 750/740/730/660, Mali-G715/G710/G78, Samsung Xclipse 940/920 AMD RDNA).
  - OpenGL ES version parsing frompacked bitfield properties (`ro.opengles.version` ➔ `OpenGL ES 3.2`).
  - Real-time Android GPU usage % and thermal sensor monitoring in the live TUI dashboard.

---

### 🎨 2. New Vendor Logos & Mobile GPU Color Themes
- Added brand block ASCII text logos for **QUALCOMM**, **ARM**, **SAMSUNG**, and **APPLE** in `--gpu` / `-g` output.
- Introduced vendor-matched ANSI color themes:
  - 🟠 **Qualcomm**: Qualcomm Orange Accent
  - 🩵 **ARM**: Bright Cyan Accent
  - 🔵 **Samsung**: Samsung Blue Accent
  - ⚪ **Apple**: Silver / White Accent

---

### 🛡️ 3. Raw-Mode Panic Protection & Signal Handling
- **Terminal Mode Safeguards**: Added top-level panic recovery and OS signal handlers (`SIGINT`, `SIGTERM`, `SIGHUP`, `SIGQUIT`) in `StartLiveDashboard`.
- **Clean Exit Guarantee**: If an unexpected panic or interrupt occurs, `KernelView` now guarantees `term.Restore` is called and alternate screen buffer reset sequences (`\033[?25h\033[?1049l\033[0m`) are sent, preventing raw-mode diagonal text cascades on Termux.
- **Worker Goroutine Panic Recovery**: All concurrent process readers in `GetProcessList()` and `LiveTracker.GetMetrics()` are wrapped in `defer func() { _ = recover() }()`, safely handling Android SELinux `/proc` permission blocks.
- **Process Recursion Guard**: Prevents infinite loops when traversing Android parent process trees in `getLinuxTerminal()`.

---

### ⚡ 4. Pre-Compiled Regex Performance Boost
- Extracted inline `regexp.MustCompile` calls across ANSI stripping (`stripAnsi`), shell version parsing, and network ping parsers into package-level pre-compiled `var` declarations.
- Eliminates unnecessary heap allocations during 500ms live dashboard ticks.

---

### 🧪 5. Android Environment Mocking
- Added `-m android` / `--mock android` to simulate a complete Qualcomm Snapdragon + Adreno 730 Android Termux setup on any desktop PC:
  ```bash
  kernelview -m android
  kernelview -m android --gpu
  ```

---

## 🛠️ Installation & Upgrade

### Binary Download
Download pre-compiled binaries for Linux, Android (Termux), macOS, and Windows from the Releases page.

### Building from Source
```bash
git clone https://github.com/codedbysoumyajit/KernelView-Go.git
cd KernelView-Go
go build -o kernelview main.go
./kernelview
```

---

## 📜 Full Changelog

- `feat(android)`: implement SoC model mapping and ARM CPU frequency scanner
- `feat(gpu)`: add Qualcomm Adreno, ARM Mali, Samsung Xclipse, and PowerVR GPU detection
- `feat(graphics)`: parse OpenGL ES bitfield version properties and Vulkan hardware properties
- `feat(theme)`: add ASCII logos and color palettes for Qualcomm, ARM, Samsung, and Apple
- `fix(tui)`: add OS signal trapping and top-level panic recovery to safely restore terminal raw mode
- `fix(goroutine)`: wrap process scanner goroutines in panic recover blocks
- `perf(regex)`: pre-compile regex patterns at package level to optimize GC during live updates
- `feat(mock)`: add `-m android` simulation flag
