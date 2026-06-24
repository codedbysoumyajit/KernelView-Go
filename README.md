# 🚀 KernelView Go

<p align="center">
  <img src="https://img.shields.io/badge/Language-Go-00ADD8?style=for-the-badge&logo=go" alt="Go" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="MIT License" />
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=for-the-badge" alt="Platforms" />
  <a href="https://goreportcard.com/report/github.com/codedbysoumyajit/KernelView-Go">
    <img src="https://goreportcard.com/badge/github.com/codedbysoumyajit/KernelView-Go?style=for-the-badge" alt="Go Report Card" />
  </a>
</p>

**KernelView Go** is an aesthetic, blazingly fast system information fetcher and real-time terminal dashboard. It is a complete rewrite of the original Python-based [KernelView](https://github.com/codedbysoumyajit/KernelView), leveraging Go's native compilation, lower memory footprint, and concurrency to deliver near-instant fetches and smooth live system telemetry with minimal CPU overhead.

---

## ✨ Key Features

### 🎨 1. Distro-Branded CLI Fetch (Fastfetch/Neofetch style)
* **High-Fidelity ASCII Art**: Recreated authentic ASCII logos matching `fastfetch` for major operating systems and Linux distributions.
* **Distro-Aware Themes**: Automatically detects your system's operating system/distribution and styles key labels, borders, and dividers using the distro's primary colors (e.g., Green for Manjaro, Cyan for Arch, Orange for Ubuntu, Crimson for Debian).
* **Side-by-Side Logo Layout**: Displays ASCII art side-by-side with system specs. Automatically wraps or falls back to a clean vertical layout on narrow terminal widths.
* **Classic Color Grid**: Includes the iconic color grid blocks (`███`) at the bottom of standard and network reports.

### 🖥️ 2. Premium Real-Time TUI Dashboard (`-l`, `--live`)
* **Flicker-Free Render Loop**: Implemented terminal home cursor-movement rendering (`\033[H`) to draw dashboard screens in-place rather than clearing, providing butter-smooth telemetry updates.
* **Rounded Themed Windows**: Frames system boxes with elegant rounded borders (`╭─`, `╮`, `│`, `╰`, `╯`) styled in distro-specific colors.
* **Physical Keycap Buttons**: Renders shortcut indicators as physically styled, inverse-color buttons in the footer (e.g., ` Q ` Quit, ` 1 ` Dashboard).
* **Multi-Tabbed Interactive Navigation**:
  * `[1] Dashboard`: Shows overall telemetry (OS specs, Hardware info, Disk usage, dynamic Resource Monitor progress bars, and a Top Processes list).
  * `[2] Processes`: Shows an expanded process tree sorted by CPU/RAM usage.
  * `[3] Network`: Displays all active interfaces with live bandwidth speeds.
  * `[4] CPU Cores`: Renders individual real-time usage percentages for each logical CPU core in a dual-column grid.

### 🎮 3. Native GPU & Intel iGPU Telemetry
* Supports querying **integrated graphics (iGPUs)** and **dedicated cards (dGPUs)** concurrently across Linux, macOS, and Windows.
* **Intel UHD iGPU**: Dynamically reads active and maximum graphics clock speeds from `/sys/class/drm/` to calculate load ratios and displays frequency statistics (e.g., `350 MHz / 1050 MHz`).
* **AMD & Nvidia GPU**: Resolves dedicated temperature, memory limits, and workloads.

### ⚡ 4. Highly Optimized Caching Architecture
To maintain a near-zero CPU footprint, the live loop splits real-time metrics (CPU cores, memory, network transfer rates) from heavy system probes via a smart caching layer:
* **Process List**: Cached for `2.0s` (averts scanning `/proc` directories 4 times a second).
* **GPU Telemetry & Temperature**: Cached for `2.0s`.
* **Disk Space Stats**: Cached for `5.0s`.
* **Network Interface Socket list**: Cached for `10.0s`.
* **Uptime**: Boot epoch resolved once on start; subsequent frames calculate time delta locally to bypass syscall overhead.

---

## 🗂️ Supported Systems & Distributions

| Linux Distributions | Apple macOS | Microsoft Windows |
| :--- | :--- | :--- |
| Arch Linux, Manjaro, Ubuntu, Debian, Fedora, CentOS, Alpine, Android (Termux), Gentoo, RedHat (RHEL), OpenSUSE, Linux Mint | macOS (Apple Silicon & Intel) | Windows 10 & 11 (Cmd, PowerShell, Terminal) |

---

## ⌨️ CLI Modes & Flags

```
╭────────────────────────────────────────────────────────╮
│   KernelView Go - System Fetch & Live Dashboard        │
│   Version: v0.1.0-alpha (Detected: manjaro)            │
╰────────────────────────────────────────────────────────╯

USAGE:
  kernelview [flags]

DISPLAY MODES:
  -l, --live         Start real-time TUI dashboard (interactive, multi-tab)
  -p, --process      Display static list of running processes
  -n, --network      Display detailed static network interfaces & stats
  [default]          Display system fetch (fastfetch-like distro ASCII logo & specs)

CONFIGURATION OPTIONS:
  -f, --fast         Run system fetch in fast mode (skips slow subsystem checks)
  --json             Output information as structured JSON
  --no-color         Disable all ANSI color formatting

INFO FLAGS:
  -v, --version      Print version and build information
  -h, --help         Print this aesthetic help menu

TUI INTERACTIVE KEYS:
  [1] Dashboard Tab   [2] Processes Tab   [3] Network Tab   [4] CPU Cores Tab   [Q] Quit

EXAMPLES:
  $ kernelview --live            # Open the live TUI dashboard
  $ kernelview -p --json         # Fetch running processes in JSON format
  $ kernelview -f                # Fast system fetch with ASCII logo
```

---

## 🏎️ Speed & Efficiency Performance Benchmarks

Pure Go system lookups (directly parsing `/proc/meminfo`, dpkg/pacman package counts, and sysfs paths) deliver near-instant results, outperforming predecessor and alternative script-based fetches:

| Mode / Tool | Execution Latency | Speedup vs Python | CPU Usage (TUI Loop) |
| :--- | :--- | :--- | :--- |
| **KernelView Go (Default Mode)** | **`0.113s`** | **~7.3x faster** | N/A (Static Fetch) |
| **KernelView Go (Fast Mode)** | **`0.046s`** | **~18.0x faster** | N/A (Static Fetch) |
| **Process List (`-p`)** | **`0.206s`** | **~1.5x faster** | N/A (Static Fetch) |
| **Network Info (`-n`)** | **`0.319s`** | **~2.5x faster** | N/A (Static Fetch) |
| **Live Dashboard (`-l`)** | **Smooth 2 Hz** | **Smooth Telemetry** | **< 1.0% Core Load** |

> [!TIP]
> The performance numbers are gathered on an Intel i7 CPU running Arch Linux. Enabling **Fast Mode (`-f`)** skips slower checks like checking multiple package managers and network ping latency, making execution almost instant.

---

## 💻 Installation

### From Source
1. **Prerequisites**: Ensure you have [**Go (1.21 or later)**](https://go.dev/dl/) installed.
2. **Clone and Build**:
   ```bash
   git clone https://github.com/codedbysoumyajit/KernelView-Go.git
   cd KernelView-Go
   go build -o kernelview main.go
   ```
3. **Move to System PATH**:
   ```bash
   sudo mv kernelview /usr/local/bin/  # Linux / macOS
   ```

### Cross-Compiling (Build for other platforms)
You can build binaries for different systems from your current machine:
```bash
# Build for Linux (amd64)
GOOS=linux GOARCH=amd64 go build -o kernelview-linux-amd64 main.go

# Build for macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o kernelview-darwin-arm64 main.go

# Build for Windows (amd64)
GOOS=windows GOARCH=amd64 go build -o kernelview-windows.exe main.go
```

---

## 🛠️ Troubleshooting & FAQ

> [!NOTE]
> **No Color Output**: If your terminal doesn't support ANSI colors or you want plain text (e.g. for piping output to a text file), pass the `--no-color` flag, or set the environment variable `NO_COLOR=1`.

> [!IMPORTANT]
> **Intel iGPU Telemetry Missing**: Intel iGPU frequency stats require access to sysfs files like `/sys/class/drm/card0/gt_act_freq_mhz`. If you are running inside a heavily sandboxed environment or flatpak, ensure KernelView Go has the appropriate system permissions to read `/sys/class/drm/`.

---

## 📄 License

This project is licensed under the **MIT License**. See the [LICENSE](file:///home/soumyajit/Projects/KernelView-Go/LICENSE) file for details.
