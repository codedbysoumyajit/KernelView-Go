
# KernelView Go 🚀

[![Go Report Card](https://goreportcard.com/badge/github.com/codedbysoumyajit/KernelView-Go)](https://goreportcard.com/report/github.com/codedbysoumyajit/KernelView-Go) **KernelView Go** is a fast and modern command-line tool designed to display essential system hardware and software information concisely. It's a complete rewrite of the original Python-based [KernelView](https://github.com/codedbysoumyajit/KernelView), leveraging the power and performance of Go.



---

## Why Go? 🤔

The primary motivation for rewriting KernelView in Go was **performance**. While the Python version was functional, it suffered from slower execution times inherent in interpreted languages, especially when shelling out for system information.

Go offers several advantages:
* **Speed:** Compiled Go code runs significantly faster, providing near-instantaneous results, especially in the optimized "fast" mode.
* **Concurrency:** Go's built-in concurrency makes it easy to gather multiple pieces of information simultaneously, further reducing execution time.
* **Single Binary:** Go compiles to a single, statically linked executable with no external dependencies, making distribution and installation much simpler.

---

## Features ✨

KernelView Go provides a clean overview of your system, including:

* **System:** OS, Kernel, Virtualization (if applicable), Uptime, Shell, Terminal
* **Hardware:** CPU Model, GPU Model, RAM Usage
* **Network:** Hostname, IP Address
* **Storage:** Disk Usage, Swap Usage
* **Display:** Resolution, Desktop Environment, Window Manager
* **Software:** Detected Packages (normal mode only), Installed Programming Languages (normal mode only), Go Version
* **CPU Stats:** Cores/Threads, Clock Speed, Current Usage (normal mode only), Temperature (normal mode only)
* **Other:** System Locale, Open Ports (normal mode only)

It features two operational modes:
1.  **Normal Mode:** Performs a comprehensive scan, including potentially slower operations like package counting and CPU/network usage monitoring.
2.  **Fast Mode (`-f`, `--fast`):** Skips the slower checks to provide essential hardware and OS information almost instantly, comparable to highly optimized tools like `fastfetch`.

---

## Performance Benchmarks ⚡

KernelView Go's fast mode (-f) is designed for near-instantaneous results, significantly outperforming its Python predecessor and other popular tools:
 * vs [KernelView Python](https://github.com/codedbysoumyajit/KernelView): ~ 33x faster
 * vs Neofetch: ~ 26x faster
 * vs Screenfetch: ~ 34x faster
 * vs Fastfetch: ~ 4.6x faster

*(Benchmarks run on archlinux (proot) and termux native. Tested using **time** command)*

---

 ## Installation 💻

**Current Status:** Alpha 🌱

KernelView Go is currently in the alpha stage. Installation via popular package managers (like Homebrew, APT, DNF, etc.) is planned for future releases.

For now, please build from source:

1.  **Prerequisites:** Ensure you have [**Go (1.25 or later)**](https://go.dev/dl/) installed.
2.  **Clone the repository:**
    ```bash
    git clone https://github.com/your-username/KernelView-Go.git # Replace with your repo URL
    cd KernelView-Go
    ```
3.  **Build:**
    ```bash
    go build -o kernelview .
    ```
    This will create the `kernelview` executable in the current directory.
4.  **Move to PATH:** You can move the executable to a directory in your system's PATH for easier access:
    ```bash
    sudo mv kernelview /usr/local/bin/ # Example for Linux/macOS
    ```

---

## Usage ⌨️

Run the compiled executable:

* **Normal Mode (Comprehensive Scan):**
    ```bash
    kernelview
    ```

* **Fast Mode (Quick Scan):**
    ```bash
    kernelview --fast
    # OR
    kernelview -f
    ```

* **Help:**
    ```bash
    kernelview --help
    # OR
    kernelview -h
    ```

---

## Contributing 🤝

Contributions are welcome! Please feel free to open an issue or submit a pull request for bug fixes, feature suggestions, or performance improvements.

---

## License 📄

This project is licensed under the **MIT License**. See the `LICENSE` file for details.
