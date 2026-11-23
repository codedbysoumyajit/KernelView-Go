# 🚀 KernelView Go v1.4.0-nightly Release Notes

**Release Tag**: `v1.4.0-nightly`  
**Codename**: *Hypercharged Telemetry, Kernel Diagnostics & Smart Installation*

---

## 🌟 Overview

**KernelView Go v1.4.0-nightly** is a monumental release! This update introduces deep Linux kernel inspection and security diagnostics with the brand-new **`-k` / `--kernel`** flag, overhauls the **`-l` / `--live`** interactive TUI dashboard with zero-allocation rendering, adds a real-time system log stream (**`[6] LOGS`**), supercharges fast mode (**`-f`**) down to **sub-10ms** execution, adds 67+ new distro logos (now **110+ total**), and introduces universal smart installation scripts with automated version upgrade detection and channel switching.

---

## ⚡ What's New in v1.4.0-nightly

### 1. 🐧 Brand New Flag: `-k` / `--kernel` (Deep Kernel & Security Diagnostics)
Inspect low-level kernel parameters, security mechanisms, and mitigation telemetry cleanly formatted alongside your distribution's ASCII art:

```bash
kernelview -k
# or
kernelview --kernel
```

* **Version & Build Toolchain**: Kernel release, target architecture, compiler (`gcc`/`clang` version), and preemption model (`PREEMPT_DYNAMIC`, `PREEMPT_RT`, `SMP`).
* **Sanitized Boot Command Line**: Strips bootloader noise (`BOOT_IMAGE`, lengthy UUIDs) to highlight critical runtime kernel parameters.
* **Kernel Modules & Footprint**: Loaded vs available disk modules, total module memory footprint, and top dependent kernel modules.
* **Kernel Taint Decoding**: Comprehensive bitwise decoding of kernel taint states (out-of-tree drivers, proprietary modules, forced loads) with human-readable descriptions.
* **Security & Isolation Mechanisms**: Real-time status for **Secure Boot** (UEFI / MOK), **Kernel Lockdown** mode (`none`, `integrity`, `confidentiality`), and active **LSMs** (`SELinux`, `AppArmor`, `Landlock`, `BPF`, `TOMOYO`, `Smack`).
* **Hardening & Hardware Features**: Live state for **KASLR**, **eBPF JIT** compiler hardening, CPU frequency scaling governor, clocksource, and memory page size.
* **CPU Vulnerabilities**: Live mitigation status parsed directly from `/sys/devices/system/cpu/vulnerabilities/*` (mitigations, not affected, vulnerable).
* **Kernel Updates & Reboot Detection**: Compares active release against installed kernels in `/boot` to flag pending reboot requirements.
* **Full JSON Support**: Export complete kernel diagnostics via `kernelview -k --json`.

---

### 2. 🖥️ Revamped Real-Time TUI Dashboard (`-l` / `--live`) & New `[6] LOGS` Stream
The interactive terminal dashboard has been engineered for maximum responsiveness, fluid ergonomics, and near-zero CPU footprint:

* **Brand New `[6] LOGS` Stream**: Real-time system log collector with multi-OS collectors:
  * **Linux**: Zero-overhead `/var/log/syslog` & `/var/log/messages` fast-paths, sub-300ms non-root user journal streaming (`journalctl --user`), fast boot-scoped system journal (`journalctl -b`), and `dmesg -T` fallback.
  * **macOS (Darwin)**: Direct `/var/log/system.log` tailing and compact unified logging (`log show --style compact`).
  * **Windows**: Native Event Log querying via `wevtutil qe System` with automated block parsing, falling back to PowerShell pipeline streaming.
  * **BSD / Unix**: Direct `/var/log/messages` inspection and kernel ring buffer (`dmesg`).
* **Interactive Log Severity Filter & History Scrolling**:
  * Tap `f` / `F` to cycle severity filter: `ALL` → `ERR` → `WARN+ERR` → `INFO` → `ALL`.
  * Use `↑` / `↓` Arrow Keys or `k` / `j` Vim Keys to scroll backwards/forwards through buffered log history (`g` / `G` for top/bottom).
  * Color-coded badges for `[INFO]`, `[WARN]`, `[ERR ]`, and `[DBUG]`.
* **Zero-Allocation Layout Engine**: Single-pass UTF-8 scanner in `visualLen` and `truncateAnsi` delivers **~60x faster text layout** with zero heap allocations per frame.
* **100% Flicker-Free Tab Switching**: Integrated synchronized terminal update buffering (`\033[?2026h` ... `\033[?2026l`) to eliminate screen flash and tearing during view transitions.
* **Fluid Keyboard Controls**:
  * `Tab` / `Shift+Tab`: Cycle through tabs forward and backward.
  * `←` / `→` Arrow Keys & `h` / `l` Vim Keys: Smooth directional tab switching.
  * `1`–`6` / `d`, `p`, `n`, `c`, `i`, `s` / `o`: Direct tab hotkeys.
  * `q` / `Q` / `Esc` / `Ctrl+C`: Clean terminal restoration and exit.
* **Adaptive Fast Live Mode**: Combine `-l` with `-f` (`kernelview -l -f`) to enable an adaptive, ultra-responsive 200ms telemetry cycle.

---

### 3. ⚡ Blazing Performance Overhaul

Every diagnostic flag and data pipeline has been optimized:

| Mode / Flag | Previous Benchmark | v1.4.0-nightly Benchmark | Performance Gain |
| :--- | :--- | :--- | :--- |
| **`-f` (Fast Fetch)** | ~60 ms *(cold: ~338ms)* | **~7 – 15 ms** | **~4x – 20x faster** 🚀 |
| **`-k` (Kernel Info)** | *New in v1.4.0* | **~7 – 10 ms** *(warm)* | **Instantaneous** |
| **`-g` (GPU Details)** | ~1133 ms | **~150 ms** | **~7.5x faster** 🏎️ |
| **`-p` (Process List)** | ~239 ms | **~112 ms** | **~2x faster** ⏩ |
| **Default Fetch** | ~190 ms | **~30 – 140 ms** | **~2x faster** |

* **Zero-Subprocess DRM & Udev Probing**: Replaced slow `lspci` forks in `-f` and default fetch with direct DRM sysfs connector & `/run/udev/data/+pci:*` RAM reads (<0.5ms).
* **Bypassed Heavy `host.Info()`**: Replaced heavy filesystem scans with single-pass reads of `/proc/uptime`, `/etc/os-release`, and `/proc/sys/kernel/osrelease`.
* **Kernel Worker Thread Pre-Filtering**: Filters kernel threads in `/proc` before object allocation, cutting iteration overhead by ~45%.
* **Parallelized Probing**: Asynchronous graphics APIs (OpenGL, Vulkan, OpenCL, CUDA), multi-source public IP resolution, and Wi-Fi telemetry.

---

### 4. 🎨 110+ Distro Logos & Fastfetch-Aligned Dynamic Theming
* **67+ New Distro Logos**: Added authentic ASCII art and official brand colors matching Fastfetch (Amazon Linux, KDE Linux, Proxmox VE, Armbian, DietPi, Slackware, Raspbian, Artix, Asahi, Haiku, GNU Guix, Devuan, Mageia, ArcoLinux, KaOS, Bodhi, DragonFly BSD, Solaris/illumos, Android, ChromeOS, Archcraft, BlackArch, Athena OS, Vanilla OS, OpenWrt, Trisquel, and more).
* **GPU Vendor ASCII Art**: Dedicated `cpufetch`-style ASCII art for **Intel**, **AMD Radeon**, **NVIDIA**, **Apple Silicon**, and **ARM Mali** in `-g` mode.
* **`NO_COLOR` Specification Support**: Full compliance with [no-color.org](https://no-color.org/) across all modes.

---

### 5. 📦 Universal Smart Installers (`install.sh` & `install.ps1`)
* **Existing Installation & Upgrade Detection**: Automatically detects installed copies of `kernelview`, compares versions, and prompts to upgrade in-place.
* **Pre-Release Channel Detection**: When a pre-release (beta/nightly) is detected on GitHub, the installer asks the user whether to install **Stable** or **Beta/Nightly**.
* **Piped Stdin Safe**: Safely reads from `/dev/tty` when executed via `curl ... | sh` or `irm ... | iex`, with clean non-interactive fallback for CI.
* **Broad Architecture Coverage**: Linux (glibc & musl/Alpine), macOS (Apple Silicon & Intel), BSDs (FreeBSD, OpenBSD, NetBSD, DragonFly), Android/Termux, and Windows (x64, ARM64, x86).

---

## 📥 Installation & Upgrades

### Quick Install (Linux, macOS, BSD, Android/Termux)
```bash
# Standard install / upgrade:
curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh

# Install beta/nightly channel directly:
curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh -s -- --beta
```

### Windows (PowerShell)
```powershell
# Standard install / upgrade:
irm https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.ps1 | iex

# Install beta/nightly channel directly:
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.ps1))) -Beta
```

### Go Install
```bash
go install github.com/codedbysoumyajit/KernelView-Go@v1.4.0-nightly
```

---

**Full Changelog**: https://github.com/codedbysoumyajit/KernelView-Go/compare/v1.3.3...v1.4.0-nightly
