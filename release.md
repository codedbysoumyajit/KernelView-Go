# 🚀 KernelView Go v1.4.0 Release Notes

**Release Date**: November 11, 2026  
**Codename**: *Hypercharged Telemetry & Kernel Hardening*

---

## 🌟 Overview

**KernelView Go v1.4.0** is one of our biggest updates yet! This release introduces the brand new **`-k` / `--kernel`** flag for deep Linux kernel inspection and security analysis, overhauls the **`-l` / `--live`** real-time TUI dashboard with zero-allocation rendering and flicker-free tab navigation, supercharges the **`-f` / `--fast`** mode down to **sub-10ms** execution, and brings dramatic performance gains across all diagnostic flags (`-g`, `-p`, `-n`).

---

## ⚡ What's New in v1.4.0

### 1. 🐧 Brand New Flag: `-k` / `--kernel` (Kernel & Security Diagnostics)
Explore deep kernel parameters, security mechanisms, and low-level hardware interactions cleanly formatted alongside your distribution's ASCII art:

```bash
kernelview -k
# or
kernelview --kernel
```

* **Version & Build Metadata**: Kernel release, target architecture, compiler toolchain (`gcc`/`clang` version), and preemption model (`PREEMPT_DYNAMIC`, `PREEMPT_RT`, `SMP`).
* **Cleaned Command Line**: Boot arguments parsed and sanitized, stripping bootloader noise (`BOOT_IMAGE`, lengthy UUIDs) to highlight critical runtime parameters.
* **Kernel Modules & Dependency Graph**: Live counts of loaded vs available disk modules, total module memory footprint, and top dependent kernel modules.
* **Kernel Taint Decoding**: Comprehensive bitwise decoding of kernel taint states (out-of-tree drivers, proprietary modules, forced loads) with clean descriptions.
* **Security & Isolation State**: Instant detection of Secure Boot (UEFI / MOK), Kernel Lockdown mode (`none`, `integrity`, `confidentiality`), and active Linux Security Modules (`SELinux`, `AppArmor`, `Landlock`, `BPF`, `TOMOYO`, `Smack`).
* **Hardening & Hardware Features**: Real-time status for **KASLR**, **eBPF JIT** compiler hardening, CPU frequency scaling driver/governor, active clocksource, and memory page size.
* **CPU Vulnerabilities**: Live mitigation summary parsed directly from `/sys/devices/system/cpu/vulnerabilities/*` (active mitigations, not affected, vulnerable).
* **Kernel Update & Reboot Detection**: Compares active release against installed kernels in `/boot` to flag pending reboot requirements.
* **Cross-Platform & JSON**: Fallback support on Windows, macOS, and BSD, with full `--json` compatibility (`kernelview -k --json`).

---

### 2. ⚡ Blazing Performance Overhaul

Every single flag and collection pipeline has been rigorously profiled and optimized:

| Mode / Flag | v1.3.x Benchmark | v1.4.0 Benchmark | Performance Gain |
| :--- | :--- | :--- | :--- |
| **`-f` (Fast Fetch)** | ~60 ms *(cold: ~338ms)* | **~7 – 15 ms** | **~4x – 20x faster** 🚀 |
| **`-k` (Kernel Info)** | *New in v1.4.0* | **~7 – 10 ms** *(warm)* | **Instantaneous** |
| **`-g` (GPU Details)** | ~1133 ms | **~150 ms** | **~7.5x faster** 🏎️ |
| **`-p` (Process List)** | ~239 ms | **~112 ms** | **~2x faster** ⏩ |
| **Default Fetch** | ~190 ms | **~30 – 140 ms** | **~2x faster** |

#### Key Technical Breakthroughs:
* **Direct DRM & Udev GPU Resolution**: Replaced expensive `lspci` child process forks (~25-250ms) in `-f` and default fetch with zero-subprocess DRM sysfs connector & `/run/udev/data/+pci:*` RAM lookups (<0.5ms).
* **Bypassed Heavy `host.Info()`**: Replaced gopsutil's broad filesystem scans with direct single-pass reads of `/proc/uptime`, `/etc/os-release`, and `/proc/sys/kernel/osrelease`.
* **Kernel Worker Thread Pre-Filtering**: Detects and filters empty-cmdline Linux kernel worker threads before object creation, cutting process iteration load by ~45% in both `-p` and the live dashboard.
* **Parallelized Graphics Probing**: Parallelized OpenGL, Vulkan, OpenCL, and CUDA queries, tuned `glxinfo -B` execution timeouts, and added instant pure-Go ICD JSON manifest parsing.
* **Fast Network Races**: Parallelized public IP resolution with racing DNS fallbacks, 350ms ping timeout, and async Wi-Fi telemetry in `-n`.

---

### 3. 🖥️ Revamped Real-Time TUI Dashboard (`-l` / `--live`)

The interactive terminal dashboard has been engineered for maximum responsiveness, fluid ergonomics, and near-zero CPU footprint:

* **Zero-Allocation Rendering Engine**: Replaced regex-based string measurement with a single-pass UTF-8 scanner in `visualLen` and `truncateAnsi`, achieving **~60x faster text layout** with zero heap allocations per frame.
* **100% Flicker-Free Tab Switching**: Integrated synchronized terminal update buffering (`\033[?2026h\033[2J\033[H` ... `\033[?2026l`) to eliminate screen flash and tearing during view transitions.
* **Modern Keyboard Navigation**:
  * `Tab` / `Shift+Tab`: Cycle through tabs forward and backward.
  * `←` / `→` Arrow Keys & `h` / `l` Vim Keys: Smooth directional tab switching.
  * `1`–`5` / `d`, `p`, `n`, `c`, `i`: Direct tab hotkeys.
  * `q` / `Q` / `Esc` / `Ctrl+C`: Clean terminal restoration and exit.
* **Zero-Alloc Linux Telemetry**: Single-pass zero-allocation parsing for `/proc/meminfo` and `/proc/net/dev`, saving over 150 heap allocations per tick.
* **Fixed GPU DRM CPU Spike**: Fixed an issue where systems without active DRM processes would continuously scan all `/proc/*/fdinfo` file descriptors on every tick.
* **Adaptive Fast Live Mode**: Combine `-l` with `-f` (`kernelview -l -f`) to enable an adaptive, ultra-responsive 200ms telemetry loop (default: 350ms with anti-jitter damping).

---

### 4. 🎨 GPU Vendor Logos & Dynamic Theming (`-g` / `--gpu`)
* **Dedicated Vendor ASCII Art**: Integrated authentic `cpufetch`-style ASCII logos for **Intel**, **AMD Radeon**, **NVIDIA**, **Apple Silicon**, and **ARM Mali**.
* **Hardware-Aware Palette Accents**: Automatically themes keys, headers, power states, and clock rates matching the detected vendor (e.g. Intel Cyan, AMD Ruby Red, Nvidia Green).

---

### 5. 🛡️ Standards Compliance & Quality-of-Life
* **`NO_COLOR` Specification Support**: Full compliance with [no-color.org](https://no-color.org/) — setting the `NO_COLOR` environment variable or using `--no-color` cleanly disables all ANSI styling and color blocks across every mode.
* **Color-Preserving ANSI Truncation**: Text truncated on narrow terminal windows retains its syntax highlighting and theme accents without stripping styling.
* **Distro Logo Enhancements**: Updated Linux unknown/generic distro ASCII art with an authentic Tux penguin logo.

---

## 📦 Detailed Commit Log

* `7f18bd4` feat: add new flag `-k` / `--kernel` for detailed kernel parameters, modules, security & mitigations
* `a84ce4e` feat: integrate cpufetch GPU vendor logos, vendor-based `-g` colors, and clean generic fallbacks
* `fc626eb` feat(tui): add interactive Disk/IO telemetry tab to live dashboard
* `02dfbd0` feat(telemetry): optimize `-g` and `-n` flags across GPU vendors (AMD, Nvidia, Intel, Apple, ARM)
* `8aac647` feat(telemetry): upgrade `-n` and `-g` flags with deep hardware and network diagnostics
* `453215a` perf: optimize `-g`, `-n`, `-p`, and default system fetch pipelines
* `04c4b2e` perf: optimize `-f` flag for sub-10ms execution and overhaul live dashboard telemetry
* `e684e3a` fix: harden security, fix DRM resolution detection, and resolve compiler warnings

---

## 📥 Installation & Upgrades

### Quick Install (Linux / macOS)
```bash
curl -sSL https://kernelview.codedbysoumyajit.dev/install.sh | bash
```

### Go Install
```bash
go install github.com/codedbysoumyajit/KernelView-Go@v1.4.0
```

### Ubuntu / Debian (PPA)
```bash
sudo add-apt-repository ppa:codedbysoumyajit/kernelview
sudo apt update
sudo apt install kernelview
```

### Arch Linux (AUR)
```bash
yay -S kernelview-bin
# or
paru -S kernelview-bin
```

---

**Full Changelog**: https://github.com/codedbysoumyajit/KernelView-Go/compare/v1.3.3...v1.4.0
