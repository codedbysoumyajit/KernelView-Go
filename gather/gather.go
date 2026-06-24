package gather

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemInfo holds all collected system data. Exported for use in main.
type SystemInfo struct {
	OS             string
	Host           string
	Kernel         string
	Uptime         string
	Shell          string
	CPU            string
	CoresThreads   string
	CPUSpeed       string
	CPUUsage       string // Skipped by --fast
	GPU            string
	RAM            string
	Disk           string
	Swap           string
	Hostname       string
	IPAddress      string
	OpenPorts      string // Skipped by --fast
	Locale         string
	Resolution     string
	WindowManager  string
	DE             string
	Terminal       string
	Packages       string // Skipped by --fast
	Languages      string // Skipped by --fast
	Go             string
	Virtualization string
	Temperature    string // Skipped by --fast
}

// ProcessInfo holds details for a single process (Exported)
type ProcessInfo struct {
	PID     int32
	Name    string
	CPU     float64
	RAM     uint64 // Store RAM in bytes
	RAMPerc float32
}

// NetworkInfo holds detailed network data (Exported)
type NetworkInfo struct {
	Hostname   string
	PrivateIP  string
	MACAddress string
	PublicIP   string
	ISP        string
	City       string
	Country    string
	Proxy      string
	DNSServers []string
	Ping       string // e.g., "15.2 ms"
	IOCounters string // Network I/O
}

// Struct to parse ip-api.com response
type ipAPIResponse struct {
	Status  string `json:"status"`
	Country string `json:"country"`
	City    string `json:"city"`
	ISP     string `json:"isp"`
	Query   string `json:"query"` // This holds the public IP
	Message string `json:"message"`
}

// --- Internal Helper Functions ---

func runCommand(name string, arg ...string) string {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runShellCommand(command string) string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FormatBytes formats a byte count into a human-readable string (Exported)
func FormatBytes(b uint64) string {
	if b > (1 << 30) {
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	}
	if b > (1 << 20) {
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	}
	if b > (1 << 10) {
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

// --- Gathering Functions ---

func getHostModel() string {
	switch runtime.GOOS {
	case "linux":
		if content, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_name"); err == nil {
			name := strings.TrimSpace(string(content))
			if contentVer, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_version"); err == nil {
				ver := strings.TrimSpace(string(contentVer))
				if name != "" && ver != "" && ver != "None" && ver != "System Version" {
					return fmt.Sprintf("%s %s", name, ver)
				}
			}
			if name != "" {
				return name
			}
		}
		if content, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
			name := strings.TrimSpace(string(content))
			if name != "" {
				return name
			}
		}
		if content, err := os.ReadFile("/sys/firmware/devicetree/base/model"); err == nil {
			name := strings.TrimSpace(string(content))
			if name != "" {
				return name
			}
		}
	case "darwin":
		cmd := exec.Command("sysctl", "-n", "hw.model")
		if out, err := cmd.Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	case "windows":
		cmd := exec.Command("wmic", "computersystem", "get", "model")
		if out, err := cmd.Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) > 1 {
				return strings.TrimSpace(lines[1])
			}
		}
	}
	return ""
}

func getLinuxUptime() (string, error) {
	content, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(content))
	if len(fields) < 1 {
		return "", fmt.Errorf("invalid uptime format")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", err
	}

	uptimeDuration := time.Duration(secs) * time.Second
	days := int(uptimeDuration.Hours() / 24)
	hours := int(uptimeDuration.Hours()) % 24
	minutes := int(uptimeDuration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d days, %d hours, %d mins", days, hours, minutes), nil
	} else if hours > 0 {
		return fmt.Sprintf("%d hours, %d mins", hours, minutes), nil
	}
	return fmt.Sprintf("%d mins", minutes), nil
}

func gatherHostInfo(info *SystemInfo, wg *sync.WaitGroup) {
	defer wg.Done()

	if runtime.GOOS == "linux" {
		uptime, err := getLinuxUptime()
		if err == nil {
			info.Uptime = uptime
		}
	}

	h, err := host.Info()
	if err != nil {
		return
	}

	if info.Uptime == "" {
		uptimeDuration := time.Second * time.Duration(h.Uptime)
		days := int(uptimeDuration.Hours() / 24)
		hours := int(uptimeDuration.Hours()) % 24
		minutes := int(uptimeDuration.Minutes()) % 60
		if days > 0 {
			info.Uptime = fmt.Sprintf("%d days, %d hours, %d mins", days, hours, minutes)
		} else if hours > 0 {
			info.Uptime = fmt.Sprintf("%d hours, %d mins", hours, minutes)
		} else {
			info.Uptime = fmt.Sprintf("%d mins", minutes)
		}
	}

	info.OS = getOSInfo()
	info.Host = getHostModel()
	kernelName := h.Platform
	if kernelName == "windows" {
		kernelName = "Windows NT"
	}
	info.Kernel = fmt.Sprintf("%s %s", strings.Title(kernelName), h.KernelVersion)
	info.Hostname, _ = os.Hostname()
}

func gatherCPUInfo(info *SystemInfo, wg *sync.WaitGroup, isFast bool) {
	defer wg.Done()

	cpuStats, err := cpu.Info()
	if err == nil && len(cpuStats) > 0 {
		info.CPU = cpuStats[0].ModelName
		mhz := cpuStats[0].Mhz
		if mhz > 1000 {
			info.CPUSpeed = fmt.Sprintf("%.2f GHz", mhz/1000.0)
		} else {
			info.CPUSpeed = fmt.Sprintf("%.0f MHz", mhz)
		}
	} else {
		info.CPU = "Unknown Processor"
	}

	cores, _ := cpu.Counts(false)
	threads, _ := cpu.Counts(true)
	info.CoresThreads = fmt.Sprintf("%d/%d", cores, threads)

	if !isFast {
		percentages, err := cpu.Percent(100*time.Millisecond, false)
		if err == nil && len(percentages) > 0 {
			info.CPUUsage = fmt.Sprintf("%.1f%%", percentages[0])
		} else {
			info.CPUUsage = "N/A"
		}
	}
}

func getLinuxMemoryInfo() (ram string, swap string, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	memMap := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(parts[1])
			valStr = strings.TrimSuffix(valStr, " kB")
			val, err := strconv.ParseUint(valStr, 10, 64)
			if err == nil {
				memMap[key] = val * 1024 // convert to bytes
			}
		}
	}

	total := memMap["MemTotal"]
	if total == 0 {
		return "", "", fmt.Errorf("invalid MemTotal")
	}

	free := memMap["MemFree"]
	buffers := memMap["Buffers"]
	cached := memMap["Cached"]
	available, ok := memMap["MemAvailable"]

	var used uint64
	if ok {
		used = total - available
	} else {
		used = total - free - buffers - cached
	}

	usedPercent := (float64(used) / float64(total)) * 100
	usedGB := float64(used) / (1 << 30)
	totalGB := float64(total) / (1 << 30)
	ram = fmt.Sprintf("%.1fGB / %.1fGB (%.0f%%)", usedGB, totalGB, usedPercent)

	swapTotal := memMap["SwapTotal"]
	if swapTotal > 0 {
		swapFree := memMap["SwapFree"]
		swapUsed := swapTotal - swapFree
		swapUsedPercent := (float64(swapUsed) / float64(swapTotal)) * 100
		swapUsedGB := float64(swapUsed) / (1 << 30)
		swapTotalGB := float64(swapTotal) / (1 << 30)
		swap = fmt.Sprintf("%.1fGB / %.1fGB (%.1f%%)", swapUsedGB, swapTotalGB, swapUsedPercent)
	} else {
		swap = "None"
	}

	return ram, swap, nil
}

func gatherMemoryInfo(info *SystemInfo, wg *sync.WaitGroup) {
	defer wg.Done()

	if runtime.GOOS == "linux" {
		ram, swap, err := getLinuxMemoryInfo()
		if err == nil {
			info.RAM = ram
			info.Swap = swap
			return
		}
	}

	// Fallback to gopsutil
	v, err := mem.VirtualMemory()
	if err == nil {
		usedGB := float64(v.Used) / (1 << 30)
		totalGB := float64(v.Total) / (1 << 30)
		info.RAM = fmt.Sprintf("%.1fGB / %.1fGB (%.0f%%)", usedGB, totalGB, v.UsedPercent)
	}
	s, err := mem.SwapMemory()
	if err == nil && s.Total > 0 {
		usedGB := float64(s.Used) / (1 << 30)
		totalGB := float64(s.Total) / (1 << 30)
		info.Swap = fmt.Sprintf("%.1fGB / %.1fGB (%.1f%%)", usedGB, totalGB, s.UsedPercent)
	} else {
		info.Swap = "None"
	}
}

func parseOSRelease() map[string]string {
	fields := make(map[string]string)
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fields
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
				val = val[1 : len(val)-1]
			} else if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") {
				val = val[1 : len(val)-1]
			}
			fields[key] = val
		}
	}
	return fields
}

func getOSInfo() string {
	switch runtime.GOOS {
	case "linux":
		fields := parseOSRelease()
		if pretty, ok := fields["PRETTY_NAME"]; ok && pretty != "" {
			return pretty
		}
		if name, ok := fields["NAME"]; ok && name != "" {
			if version, ok := fields["VERSION"]; ok && version != "" {
				return name + " " + version
			}
			return name
		}
		platform, _, version, _ := host.PlatformInformation()
		if platform != "" && version != "" {
			return fmt.Sprintf("%s %s", platform, version)
		}
	case "windows":
		productName := runShellCommand("(Get-CimInstance Win32_OperatingSystem).Caption")
		buildNumber := runShellCommand("(Get-CimInstance Win32_OperatingSystem).BuildNumber")
		if productName != "" {
			productName = strings.TrimSpace(strings.Replace(productName, "Microsoft ", "", 1))
			if buildNumber != "" {
				return fmt.Sprintf("%s (Build %s)", productName, buildNumber)
			}
			return productName
		}
	case "darwin":
		productVersion := runCommand("sw_vers", "-productVersion")
		buildVersion := runCommand("sw_vers", "-buildVersion")
		if productVersion != "" {
			return fmt.Sprintf("macOS %s (%s)", productVersion, buildVersion)
		}
	}
	h, _ := host.Info()
	return fmt.Sprintf("%s %s", h.Platform, h.PlatformVersion)
}

func getShell() string {
	shellPath := ""
	if runtime.GOOS != "windows" {
		shellPath = os.Getenv("SHELL")
		if shellPath == "" {
			return "Unknown"
		}
	} else {
		if os.Getenv("PSModulePath") != "" {
			shellPath = "powershell"
		} else if os.Getenv("ComSpec") != "" {
			shellPath = "cmd"
		} else if os.Getenv("WT_SESSION") != "" {
			return "Windows Terminal"
		} else {
			return "Unknown"
		}
	}

	shellName := shellPath[strings.LastIndex(shellPath, "/")+1:]
	shellName = strings.ToLower(shellName)
	shellName = strings.TrimSuffix(shellName, ".exe")

	var version string
	switch shellName {
	case "bash", "zsh", "fish":
		out := runCommand(shellPath, "--version")
		if out != "" {
			firstLine := strings.Split(out, "\n")[0]
			re := regexp.MustCompile(`(\d+\.\d+(\.\d+)?)`)
			version = re.FindString(firstLine)
		}
	case "powershell":
		version = runShellCommand("$PSVersionTable.PSVersion.Major")
	}

	titleName := strings.Title(shellName)
	if version != "" {
		return fmt.Sprintf("%s %s", titleName, version)
	}
	return titleName
}

func getLinuxGPU() string {
	cmd := exec.Command("lspci", "-mm")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var gpus []string
	seen := make(map[string]bool)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "vga") || strings.Contains(lower, "3d") || strings.Contains(lower, "display") {
			parts := strings.Split(line, "\"")
			vendor := ""
			device := ""
			if len(parts) > 3 {
				vendor = parts[3]
			}
			if len(parts) > 5 {
				device = parts[5]
			}
			device = strings.TrimPrefix(device, "[")
			device = strings.TrimSuffix(device, "]")
			
			gpuName := ""
			if vendor != "" && device != "" {
				gpuName = vendor + " " + device
			} else if vendor != "" {
				gpuName = vendor
			} else if device != "" {
				gpuName = device
			}

			if gpuName != "" && !seen[gpuName] {
				seen[gpuName] = true
				gpus = append(gpus, gpuName)
			}
		}
	}
	if len(gpus) > 0 {
		return strings.Join(gpus, ", ")
	}
	return ""
}

func getGPUInfo() string {
	switch runtime.GOOS {
	case "windows":
		output := runShellCommand("(Get-CimInstance Win32_VideoController).Caption")
		lines := strings.Split(output, "\n")
		var gpus []string
		for _, line := range lines {
			t := strings.TrimSpace(line)
			if t != "" {
				gpus = append(gpus, t)
			}
		}
		if len(gpus) > 0 {
			return strings.Join(gpus, ", ")
		}
		return "Unknown"
	case "linux":
		return getLinuxGPU()
	case "darwin":
		output := runShellCommand("system_profiler SPDisplaysDataType | grep 'Chipset Model' | cut -d ':' -f2")
		lines := strings.Split(output, "\n")
		var gpus []string
		for _, line := range lines {
			t := strings.TrimSpace(line)
			if t != "" {
				gpus = append(gpus, t)
			}
		}
		if len(gpus) > 0 {
			return strings.Join(gpus, ", ")
		}
		return "Unknown"
	}
	return "Unknown"
}

func getOpenPorts() string {
	conns, err := psnet.Connections("tcp")
	if err != nil {
		return "Unknown"
	}
	portSet := make(map[string]struct{})
	for _, conn := range conns {
		if conn.Status == "LISTEN" {
			portSet[strconv.Itoa(int(conn.Laddr.Port))] = struct{}{}
		}
	}
	if len(portSet) == 0 {
		return "None"
	}
	ports := make([]int, 0, len(portSet))
	for pStr := range portSet {
		p, _ := strconv.Atoi(pStr)
		ports = append(ports, p)
	}
	sort.Ints(ports)
	var portStrings []string
	for _, p := range ports {
		portStrings = append(portStrings, strconv.Itoa(p))
	}
	limit := 8
	if len(portStrings) > limit {
		return strings.Join(portStrings[:limit], ", ") + "..."
	}
	return strings.Join(portStrings, ", ")
}

func getInstalledLanguages() string {
	langs := []string{"Python", "Go", "Node", "Rust", "Java", "Ruby", "PHP"}
	cmds := map[string]string{
		"Python": "python3", "Go": "go", "Node": "node", "Rust": "rustc", "Java": "java",
		"Ruby": "ruby", "PHP": "php",
	}
	var installed []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, lang := range langs {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			if _, err := exec.LookPath(cmds[l]); err == nil {
				mu.Lock()
				installed = append(installed, l)
				mu.Unlock()
			}
		}(lang)
	}
	wg.Wait()
	sort.Strings(installed)
	if len(installed) == 0 {
		return "None"
	}
	return strings.Join(installed, ", ")
}

func getIPAddress() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		addrs, err := net.InterfaceAddrs()
		if err == nil {
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String()
					}
				}
			}
		}
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func getLinuxResolution() string {
	if os.Getenv("DISPLAY") == "" {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			files, err := os.ReadDir("/sys/class/drm")
			if err == nil {
				for _, f := range files {
					if strings.HasPrefix(f.Name(), "card") && !strings.Contains(f.Name(), "-") {
						outputs, err := os.ReadDir("/sys/class/drm/" + f.Name())
						if err == nil {
							for _, out := range outputs {
								if strings.Contains(out.Name(), "-") {
									modeFile := "/sys/class/drm/" + f.Name() + "/" + out.Name() + "/modes"
									if content, err := os.ReadFile(modeFile); err == nil {
										lines := strings.Split(string(content), "\n")
										if len(lines) > 0 && lines[0] != "" {
											return strings.TrimSpace(lines[0])
										}
									}
								}
							}
						}
					}
				}
			}
			return "Wayland (Generic)"
		}
		return "Headless"
	}
	cmd := exec.Command("xrandr", "--current")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "*") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func getResolution() string {
	switch runtime.GOOS {
	case "windows":
		output := runShellCommand("(Get-CimInstance Win32_VideoController).CurrentHorizontalResolution,(Get-CimInstance Win32_VideoController).CurrentVerticalResolution -join 'x'")
		if output != "" {
			return output
		}
	case "linux":
		return getLinuxResolution()
	case "darwin":
		output := runShellCommand("system_profiler SPDisplaysDataType | grep Resolution | awk '{print $2\"x\"$4}'")
		return strings.TrimSpace(output)
	}
	return "Unknown"
}

func getTerminal() string {
	termProg := os.Getenv("TERM_PROGRAM")
	if termProg != "" {
		termProg = strings.TrimSuffix(termProg, ".app")
		termProg = strings.Replace(termProg, "iTerm", "iTerm2", 1)
		return strings.Title(termProg)
	}
	term := os.Getenv("TERM")
	if term != "" && term != "xterm-256color" && term != "screen" {
		return term
	}
	return "Unknown"
}

func getLinuxWM() string {
	desktopSession := os.Getenv("DESKTOP_SESSION")
	if desktopSession != "" {
		lowerSession := strings.ToLower(desktopSession)
		if strings.Contains(lowerSession, "gnome") {
			return "Mutter (X11)"
		}
		if strings.Contains(lowerSession, "kde") || strings.Contains(lowerSession, "plasma") {
			return "KWin (X11)"
		}
		if strings.Contains(lowerSession, "xfce") {
			return "Xfwm4"
		}
		if strings.Contains(lowerSession, "cinnamon") {
			return "Muffin"
		}
		if strings.Contains(lowerSession, "mate") {
			return "Marco"
		}
		if strings.Contains(lowerSession, "lxqt") {
			return "Openbox"
		}
		return strings.Title(desktopSession)
	}
	cmd := exec.Command("wmctrl", "-m")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Name:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			}
		}
	}
	return "Unknown"
}

func getWindowManager() string {
	if runtime.GOOS == "linux" {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			session := os.Getenv("XDG_SESSION_TYPE")
			if session == "wayland" {
				currentDesktop := os.Getenv("XDG_CURRENT_DESKTOP")
				switch strings.ToLower(currentDesktop) {
				case "gnome":
					return "Mutter (Wayland)"
				case "kde":
					return "KWin (Wayland)"
				case "sway":
					return "Sway"
				case "wlroots":
					return "wlroots based"
				}
				return "Wayland"
			}
		}
		return getLinuxWM()
	} else if runtime.GOOS == "windows" {
		return "DWM"
	} else if runtime.GOOS == "darwin" {
		return "Quartz Compositor"
	}
	return "Unknown"
}

func getSystemLocale() string {
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = os.Getenv("LC_ALL")
	}
	if locale != "" {
		return strings.Split(locale, ".")[0]
	}
	if runtime.GOOS == "windows" {
		return runShellCommand("(Get-Culture).Name")
	}
	return "Unknown"
}

func getDesktopEnvironment() string {
	de := os.Getenv("XDG_CURRENT_DESKTOP")
	if de == "" {
		de = os.Getenv("DESKTOP_SESSION")
	}
	de = strings.Replace(de, "plasmawayland", "Plasma (Wayland)", 1)
	de = strings.Replace(de, "plasma", "Plasma (X11)", 1)
	return strings.Title(de)
}

func getPackageCounts() string {
	var results []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	type pmCheck struct {
		name string
		f    func() int
	}

	checks := []pmCheck{
		{"Pacman", func() int {
			files, err := os.ReadDir("/var/lib/pacman/local")
			if err != nil {
				return 0
			}
			count := 0
			for _, f := range files {
				if f.IsDir() && !strings.HasPrefix(f.Name(), ".") {
					count++
				}
			}
			return count
		}},
		{"APT", func() int {
			file, err := os.Open("/var/lib/dpkg/status")
			if err != nil {
				return 0
			}
			defer file.Close()
			count := 0
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "Package: ") {
					count++
				}
			}
			return count
		}},
		{"Flatpak", func() int {
			count := 0
			if dirs, err := os.ReadDir("/var/lib/flatpak/app"); err == nil {
				for _, d := range dirs {
					if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
						count++
					}
				}
			}
			home := os.Getenv("HOME")
			if home != "" {
				if dirs, err := os.ReadDir(home + "/.local/share/flatpak/app"); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
							count++
						}
					}
				}
			}
			return count
		}},
		{"Snap", func() int {
			files, err := os.ReadDir("/var/lib/snapd/snaps")
			if err != nil {
				return 0
			}
			count := 0
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".snap") {
					count++
				}
			}
			return count
		}},
		{"Brew", func() int {
			count := 0
			for _, path := range []string{"/opt/homebrew/Cellar", "/usr/local/Cellar", "/home/linuxbrew/.linuxbrew/Cellar"} {
				if dirs, err := os.ReadDir(path); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
							count++
						}
					}
				}
			}
			for _, path := range []string{"/opt/homebrew/Caskroom", "/usr/local/Caskroom", "/home/linuxbrew/.linuxbrew/Caskroom"} {
				if dirs, err := os.ReadDir(path); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
							count++
						}
					}
				}
			}
			return count
		}},
		{"RPM", func() int {
			if _, err := exec.LookPath("rpm"); err != nil {
				return 0
			}
			cmd := exec.Command("rpm", "-qa")
			out, err := cmd.Output()
			if err != nil {
				return 0
			}
			lines := strings.Split(string(out), "\n")
			count := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					count++
				}
			}
			return count
		}},
	}

	for _, check := range checks {
		wg.Add(1)
		go func(c pmCheck) {
			defer wg.Done()
			count := c.f()
			if count > 0 {
				mu.Lock()
				results = append(results, fmt.Sprintf("%s (%d)", c.name, count))
				mu.Unlock()
			}
		}(check)
	}

	wg.Wait()
	sort.Strings(results)
	if len(results) == 0 {
		return "None detected"
	}
	return strings.Join(results, ", ")
}

func getDisk() string {
	d, err := disk.Usage("/")
	if err != nil {
		return "N/A"
	}
	usedGB := float64(d.Used) / (1 << 30)
	totalGB := float64(d.Total) / (1 << 30)
	return fmt.Sprintf("%.1fGB / %.1fGB (%.0f%%)", usedGB, totalGB, d.UsedPercent)
}

func getGoVersion() string {
	return runtime.Version()
}

func getVirtualization() string {
	virt, _, err := host.Virtualization()
	if err != nil || virt == "" {
		return ""
	}
	return virt
}

func getTemperatures() string {
	temps, err := host.SensorsTemperatures()
	if err != nil || len(temps) == 0 {
		return ""
	}
	for _, temp := range temps {
		lowerKey := strings.ToLower(temp.SensorKey)
		if strings.Contains(lowerKey, "core") || strings.Contains(lowerKey, "cpu") || strings.Contains(lowerKey, "package") {
			return fmt.Sprintf("%.1f °C", temp.Temperature)
		}
	}
	return fmt.Sprintf("%.1f °C", temps[0].Temperature)
}

// --- Network Specific Functions ---

func getPrimaryMACAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "N/A"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(strings.ToLower(iface.Name), "virtual") || strings.Contains(strings.ToLower(iface.Name), "docker") {
			continue
		}
		if iface.HardwareAddr.String() != "" {
			return iface.HardwareAddr.String()
		}
	}
	return "N/A"
}

func getPublicIPInfo() (ip, isp, city, country string, err error) {
	client := http.Client{
		Timeout: 400 * time.Millisecond,
	}
	resp, err := client.Get("http://ip-api.com/json/")
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to reach ip-api: %w", err)
	}
	defer resp.Body.Close()

	var apiResp ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", "", "", "", fmt.Errorf("failed to parse ip-api response: %w", err)
	}

	if apiResp.Status != "success" {
		return "", "", "", "", fmt.Errorf("ip-api error: %s", apiResp.Message)
	}

	return apiResp.Query, apiResp.ISP, apiResp.City, apiResp.Country, nil
}

func getProxyInfo() string {
	httpProxy := os.Getenv("HTTP_PROXY")
	httpsProxy := os.Getenv("HTTPS_PROXY")
	if httpProxy != "" {
		return fmt.Sprintf("HTTP: %s", httpProxy)
	}
	if httpsProxy != "" {
		return fmt.Sprintf("HTTPS: %s", httpsProxy)
	}
	return "None"
}

func getDNSServers() []string {
	var dnsServers []string
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		content, err := os.ReadFile("/etc/resolv.conf")
		if err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "nameserver") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						dnsServers = append(dnsServers, parts[1])
					}
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		return []string{"(Check ipconfig /all)"}
	}
	if len(dnsServers) == 0 {
		return []string{"N/A"}
	}
	if len(dnsServers) > 3 {
		dnsServers = append(dnsServers[:3], "...")
	}
	return dnsServers
}

func getSystemPingTime(host string) string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "1", "-w", "400", host)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("ping", "-c", "1", "-t", "1", host)
	} else {
		cmd = exec.Command("ping", "-c", "1", "-W", "1", host)
	}
	out, err := cmd.Output()
	if err != nil {
		return "Failed"
	}

	outputStr := string(out)
	if runtime.GOOS == "windows" {
		re := regexp.MustCompile(`Average\s*=\s*(\d+)ms`)
		matches := re.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			return fmt.Sprintf("%s ms", matches[1])
		}
	} else {
		re := regexp.MustCompile(`(rtt|round-trip)\s+min/avg/max/(mdev|stddev)\s+=\s+([0-9.]+)/([0-9.]+)/([0-9.]+)/([0-9.]+)`)
		matches := re.FindStringSubmatch(outputStr)
		if len(matches) > 4 {
			return fmt.Sprintf("%s ms", matches[4])
		}
		re2 := regexp.MustCompile(`time=([0-9.]+)`)
		matches2 := re2.FindStringSubmatch(outputStr)
		if len(matches2) > 1 {
			return fmt.Sprintf("%s ms", matches2[1])
		}
	}
	return "Timeout"
}

func getIOCounters() string {
	counters, err := psnet.IOCounters(true)
	if err != nil {
		return "N/A"
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "N/A"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(strings.ToLower(iface.Name), "virtual") || strings.Contains(strings.ToLower(iface.Name), "docker") || strings.HasPrefix(iface.Name, "veth") {
			continue
		}

		for _, counter := range counters {
			if counter.Name == iface.Name {
				return fmt.Sprintf("%s (Sent: %s, Recv: %s)",
					iface.Name,
					FormatBytes(counter.BytesSent),
					FormatBytes(counter.BytesRecv))
			}
		}
	}

	if len(counters) > 0 {
		return fmt.Sprintf("All (Sent: %s, Recv: %s)",
			FormatBytes(counters[0].BytesSent),
			FormatBytes(counters[0].BytesRecv))
	}

	return "N/A"
}

// --- Main Orchestration Functions ---

// GetSystemInfo is the main exported function to collect system data.
func GetSystemInfo(isFast bool) *SystemInfo {
	info := &SystemInfo{}
	var wg sync.WaitGroup

	// --- Fast Group (Always Run) ---
	wg.Add(3)
	go gatherHostInfo(info, &wg)
	go gatherCPUInfo(info, &wg, isFast)
	go gatherMemoryInfo(info, &wg)

	// --- Fast Standalone Tasks (Always Run) ---
	fastTasks := map[string]*string{
		"Shell": &info.Shell, "GPU": &info.GPU, "Disk": &info.Disk, "IPAddress": &info.IPAddress,
		"Locale": &info.Locale, "Resolution": &info.Resolution, "WindowManager": &info.WindowManager,
		"DE": &info.DE, "Terminal": &info.Terminal, "Go": &info.Go,
		"Virtualization": &info.Virtualization,
	}
	fastTaskFuncs := map[string]func() string{
		"Shell": getShell,
		"GPU": func() string {
			if runtime.GOOS == "linux" {
				return getLinuxGPU()
			}
			return getGPUInfo()
		},
		"Disk": getDisk, "IPAddress": getIPAddress,
		"Locale": getSystemLocale,
		"Resolution": func() string {
			if runtime.GOOS == "linux" {
				return getLinuxResolution()
			}
			return getResolution()
		},
		"WindowManager": func() string {
			if runtime.GOOS == "linux" {
				return getLinuxWM()
			}
			return getWindowManager()
		},
		"DE": getDesktopEnvironment, "Terminal": getTerminal, "Go": getGoVersion,
		"Virtualization": getVirtualization,
	}
	for key, Ptr := range fastTasks {
		wg.Add(1)
		go func(p *string, f func() string) {
			defer wg.Done()
			*p = f()
		}(Ptr, fastTaskFuncs[key])
	}

	// --- Conditional Slow Tasks (Only run if !isFast) ---
	if !isFast {
		slowTasks := map[string]*string{
			"OpenPorts":   &info.OpenPorts,
			"Packages":    &info.Packages,
			"Languages":   &info.Languages,
			"Temperature": &info.Temperature,
		}
		slowTaskFuncs := map[string]func() string{
			"OpenPorts":   getOpenPorts,
			"Packages":    getPackageCounts,
			"Languages":   getInstalledLanguages,
			"Temperature": getTemperatures,
		}
		for key, Ptr := range slowTasks {
			wg.Add(1)
			go func(p *string, f func() string) {
				defer wg.Done()
				*p = f()
			}(Ptr, slowTaskFuncs[key])
		}
	}

	wg.Wait()
	return info
}

// GetProcessList fetches information about running processes. (Exported)
func GetProcessList() ([]ProcessInfo, error) {
	allProcs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to get processes: %w", err)
	}

	var wg sync.WaitGroup
	results := make(chan ProcessInfo, len(allProcs))
	sem := make(chan struct{}, 32) // Limit concurrency to avoid FD exhaustion

	// Step 1: Prime the CPU percent calculation by calling it once on all processes.
	for _, p := range allProcs {
		wg.Add(1)
		go func(proc *process.Process) {
			defer wg.Done()
			sem <- struct{}{}
			_, _ = proc.CPUPercent()
			<-sem
		}(p)
	}
	wg.Wait()

	// Step 2: Sleep to allow CPU usage to accumulate.
	time.Sleep(100 * time.Millisecond)

	// Step 3: Call CPUPercent on the same instances to get the actual CPU usage.
	for _, p := range allProcs {
		wg.Add(1)
		go func(proc *process.Process) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			pid := proc.Pid
			name, err := proc.Name()
			if err != nil || name == "" || pid <= 1 {
				return
			}

			// Exclude common system/idle processes to keep the list clean
			if runtime.GOOS == "linux" && (strings.HasPrefix(name, "[") || name == "systemd" || name == "init") {
				return
			}
			if name == "kernel_task" || name == "idle" {
				return
			}

			cpuPerc, err := proc.CPUPercent()
			if err != nil {
				cpuPerc = 0.0
			}
			memInfo, errMem := proc.MemoryInfo()
			memPerc, errMemPerc := proc.MemoryPercent()

			if errMem == nil && errMemPerc == nil && memInfo != nil {
				results <- ProcessInfo{
					PID:     pid,
					Name:    name,
					CPU:     cpuPerc,
					RAM:     memInfo.RSS,
					RAMPerc: memPerc,
				}
			}
		}(p)
	}

	wg.Wait()
	close(results)

	var processList []ProcessInfo
	for pInfo := range results {
		if pInfo.CPU > 0.05 || pInfo.RAMPerc > 0.1 {
			processList = append(processList, pInfo)
		}
	}

	sort.Slice(processList, func(i, j int) bool {
		if processList[i].CPU != processList[j].CPU {
			return processList[i].CPU > processList[j].CPU
		}
		return processList[i].RAM > processList[j].RAM
	})

	return processList, nil
}

// GetNetworkDetails fetches all network-related info (Exported)
func GetNetworkDetails() (*NetworkInfo, error) {
	info := &NetworkInfo{}
	var wg sync.WaitGroup
	var errPublicIP error

	wg.Add(3)

	go func() {
		defer wg.Done()
		info.PublicIP, info.ISP, info.City, info.Country, errPublicIP = getPublicIPInfo()
	}()

	go func() {
		defer wg.Done()
		info.Ping = getSystemPingTime("1.1.1.1")
	}()

	go func() {
		defer wg.Done()
		info.DNSServers = getDNSServers()
	}()

	// Run fast local fetches concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		info.Hostname, _ = os.Hostname()
		info.PrivateIP = getIPAddress()
		info.MACAddress = getPrimaryMACAddress()
		info.Proxy = getProxyInfo()
		info.IOCounters = getIOCounters()
	}()

	wg.Wait()

	if errPublicIP != nil {
		info.PublicIP = "Error"
		info.ISP = "Error"
		info.City = "Error"
		info.Country = "Error"
	}

	return info, nil
}

// LiveMetrics holds real-time system stats (Exported)
type LiveMetrics struct {
	Uptime      string
	CPUUsage    float64
	CPUCores    []float64 // Per-core CPU percentages (Exported)
	RAMUsed     uint64
	RAMTotal    uint64
	RAMPercent  float64
	SwapUsed    uint64
	SwapTotal   uint64
	SwapPercent float64
	DiskUsed    uint64
	DiskTotal   uint64
	DiskPercent float64
	Temperature float64
	NetRxSpeed  float64 // bytes/sec
	NetTxSpeed  float64 // bytes/sec
	NetRxTotal  uint64  // bytes
	NetTxTotal  uint64  // bytes
	NetIface    string
	Processes   []ProcessInfo
	GPUMetrics  LiveGPUMetrics // GPU telemetry details (Exported)
}

// LiveTracker tracks metrics across real-time updates (Exported)
type LiveTracker struct {
	procsMap          map[int32]*process.Process
	prevNetRx         uint64
	prevNetTx         uint64
	prevNetTime       time.Time
	mu                sync.Mutex
	bootTime          time.Time

	// Cached processes list
	cachedProcesses   []ProcessInfo
	lastProcUpdate    time.Time

	// Cached disk usage
	cachedDiskUsed    uint64
	cachedDiskTotal   uint64
	cachedDiskPercent float64
	lastDiskUpdate    time.Time

	// Cached temperature
	cachedTemp        float64
	lastTempUpdate    time.Time

	// Cached net interface name
	cachedNetIface    string
	lastIfaceUpdate   time.Time

	// Cached GPU metrics
	cachedGPUMetrics  LiveGPUMetrics
	lastGPUUpdate     time.Time
}

// NewLiveTracker creates a new tracker instance (Exported)
func NewLiveTracker() *LiveTracker {
	h, err := host.Info()
	var bTime time.Time
	if err == nil {
		bTime = time.Unix(int64(h.BootTime), 0)
	}
	return &LiveTracker{
		procsMap: make(map[int32]*process.Process),
		bootTime: bTime,
	}
}

// GetMetrics returns the calculated live metrics (Exported)
func (lt *LiveTracker) GetMetrics() (*LiveMetrics, error) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	metrics := &LiveMetrics{}

	// 1. Uptime
	if runtime.GOOS == "linux" {
		uptime, err := getLinuxUptime()
		if err == nil {
			metrics.Uptime = uptime
		}
	}
	if metrics.Uptime == "" {
		if !lt.bootTime.IsZero() {
			uptimeDuration := time.Since(lt.bootTime)
			days := int(uptimeDuration.Hours() / 24)
			hours := int(uptimeDuration.Hours()) % 24
			minutes := int(uptimeDuration.Minutes()) % 60
			if days > 0 {
				metrics.Uptime = fmt.Sprintf("%d days, %d hours, %d mins", days, hours, minutes)
			} else if hours > 0 {
				metrics.Uptime = fmt.Sprintf("%d hours, %d mins", hours, minutes)
			} else {
				metrics.Uptime = fmt.Sprintf("%d mins", minutes)
			}
		}
	}

	// 2. CPU Usage & Per-Core Percentages (real-time, queried every 500ms)
	percentages, err := cpu.Percent(0, true)
	if err == nil && len(percentages) > 0 {
		metrics.CPUCores = percentages
		var sum float64
		for _, p := range percentages {
			sum += p
		}
		metrics.CPUUsage = sum / float64(len(percentages))
	} else {
		overall, err := cpu.Percent(0, false)
		if err == nil && len(overall) > 0 {
			metrics.CPUUsage = overall[0]
		}
	}

	// 3. Memory (real-time, queried every 500ms)
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err == nil {
			memMap := make(map[string]uint64)
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					valStr := strings.TrimSpace(parts[1])
					valStr = strings.TrimSuffix(valStr, " kB")
					val, err := strconv.ParseUint(valStr, 10, 64)
					if err == nil {
						memMap[key] = val * 1024
					}
				}
			}
			file.Close()

			total := memMap["MemTotal"]
			if total > 0 {
				free := memMap["MemFree"]
				buffers := memMap["Buffers"]
				cached := memMap["Cached"]
				available, ok := memMap["MemAvailable"]

				var used uint64
				if ok {
					used = total - available
				} else {
					used = total - free - buffers - cached
				}

				metrics.RAMTotal = total
				metrics.RAMUsed = used
				metrics.RAMPercent = (float64(used) / float64(total)) * 100

				swapTotal := memMap["SwapTotal"]
				if swapTotal > 0 {
					swapFree := memMap["SwapFree"]
					metrics.SwapTotal = swapTotal
					metrics.SwapUsed = swapTotal - swapFree
					metrics.SwapPercent = (float64(metrics.SwapUsed) / float64(swapTotal)) * 100
				}
			}
		}
	}
	if metrics.RAMTotal == 0 {
		v, err := mem.VirtualMemory()
		if err == nil {
			metrics.RAMTotal = v.Total
			metrics.RAMUsed = v.Used
			metrics.RAMPercent = v.UsedPercent
		}
		s, err := mem.SwapMemory()
		if err == nil && s.Total > 0 {
			metrics.SwapTotal = s.Total
			metrics.SwapUsed = s.Used
			metrics.SwapPercent = s.UsedPercent
		}
	}

	// 4. Disk (cached for 5 seconds)
	if time.Since(lt.lastDiskUpdate) >= 5*time.Second || lt.lastDiskUpdate.IsZero() {
		d, err := disk.Usage("/")
		if err == nil {
			lt.cachedDiskTotal = d.Total
			lt.cachedDiskUsed = d.Used
			lt.cachedDiskPercent = d.UsedPercent
			lt.lastDiskUpdate = time.Now()
		}
	}
	metrics.DiskTotal = lt.cachedDiskTotal
	metrics.DiskUsed = lt.cachedDiskUsed
	metrics.DiskPercent = lt.cachedDiskPercent

	// 5. Temperature (cached for 2 seconds)
	if time.Since(lt.lastTempUpdate) >= 2*time.Second || lt.lastTempUpdate.IsZero() {
		temps, err := host.SensorsTemperatures()
		if err == nil && len(temps) > 0 {
			found := false
			for _, temp := range temps {
				lowerKey := strings.ToLower(temp.SensorKey)
				if strings.Contains(lowerKey, "core") || strings.Contains(lowerKey, "cpu") || strings.Contains(lowerKey, "package") {
					lt.cachedTemp = temp.Temperature
					found = true
					break
				}
			}
			if !found {
				lt.cachedTemp = temps[0].Temperature
			}
			lt.lastTempUpdate = time.Now()
		}
	}
	metrics.Temperature = lt.cachedTemp

	// 6. Network Rates (Iface list cached for 10 seconds, stats queried every 500ms)
	var rxTotal, txTotal uint64
	var activeIface string
	counters, err := psnet.IOCounters(true)
	if err == nil {
		if time.Since(lt.lastIfaceUpdate) >= 10*time.Second || lt.lastIfaceUpdate.IsZero() || lt.cachedNetIface == "" {
			ifaces, err := net.Interfaces()
			if err == nil {
				for _, iface := range ifaces {
					if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(strings.ToLower(iface.Name), "virtual") || strings.Contains(strings.ToLower(iface.Name), "docker") || strings.HasPrefix(iface.Name, "veth") {
						continue
					}
					for _, counter := range counters {
						if counter.Name == iface.Name {
							lt.cachedNetIface = iface.Name
							break
						}
					}
					if lt.cachedNetIface != "" {
						break
					}
				}
				lt.lastIfaceUpdate = time.Now()
			}
		}
		activeIface = lt.cachedNetIface
		for _, counter := range counters {
			if counter.Name == activeIface {
				rxTotal = counter.BytesRecv
				txTotal = counter.BytesSent
				break
			}
		}
	}

	now := time.Now()
	if !lt.prevNetTime.IsZero() {
		elapsed := now.Sub(lt.prevNetTime).Seconds()
		if elapsed > 0 {
			if rxTotal >= lt.prevNetRx {
				metrics.NetRxSpeed = float64(rxTotal-lt.prevNetRx) / elapsed
			}
			if txTotal >= lt.prevNetTx {
				metrics.NetTxSpeed = float64(txTotal-lt.prevNetTx) / elapsed
			}
		}
	}
	lt.prevNetRx = rxTotal
	lt.prevNetTx = txTotal
	lt.prevNetTime = now
	metrics.NetRxTotal = rxTotal
	metrics.NetTxTotal = txTotal
	metrics.NetIface = activeIface

	// 7. GPU Telemetry (cached for 2 seconds)
	if time.Since(lt.lastGPUUpdate) >= 2*time.Second || lt.lastGPUUpdate.IsZero() {
		gpuMetrics, err := getGPUMetrics()
		if err == nil {
			lt.cachedGPUMetrics = gpuMetrics
			lt.lastGPUUpdate = time.Now()
		}
	}
	metrics.GPUMetrics = lt.cachedGPUMetrics

	// 8. Processes (expensive, cached for 2 seconds)
	if time.Since(lt.lastProcUpdate) >= 2*time.Second || lt.lastProcUpdate.IsZero() || len(lt.cachedProcesses) == 0 {
		pids, err := process.Pids()
		if err == nil {
			currentPids := make(map[int32]bool)
			for _, pid := range pids {
				currentPids[pid] = true
			}

			for pid := range lt.procsMap {
				if !currentPids[pid] {
					delete(lt.procsMap, pid)
				}
			}

			for _, pid := range pids {
				if _, exists := lt.procsMap[pid]; !exists {
					proc, err := process.NewProcess(pid)
					if err == nil {
						lt.procsMap[pid] = proc
						_, _ = proc.CPUPercent()
					}
				}
			}

			var wg sync.WaitGroup
			sem := make(chan struct{}, 32)
			results := make(chan ProcessInfo, len(lt.procsMap))

			for _, proc := range lt.procsMap {
				wg.Add(1)
				go func(p *process.Process) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					pid := p.Pid
					name, err := p.Name()
					if err != nil || name == "" || pid <= 1 {
						return
					}

					if runtime.GOOS == "linux" && (strings.HasPrefix(name, "[") || name == "systemd" || name == "init") {
						return
					}
					if name == "kernel_task" || name == "idle" {
						return
					}

					cpuPerc, err := p.CPUPercent()
					if err != nil {
						cpuPerc = 0.0
					}
					memInfo, errMem := p.MemoryInfo()
					memPerc, errMemPerc := p.MemoryPercent()

					if errMem == nil && errMemPerc == nil && memInfo != nil {
						results <- ProcessInfo{
							PID:     pid,
							Name:    name,
							CPU:     cpuPerc,
							RAM:     memInfo.RSS,
							RAMPerc: memPerc,
						}
					}
				}(proc)
			}

			wg.Wait()
			close(results)

			var procList []ProcessInfo
			for pInfo := range results {
				procList = append(procList, pInfo)
			}

			sort.Slice(procList, func(i, j int) bool {
				if procList[i].CPU != procList[j].CPU {
					return procList[i].CPU > procList[j].CPU
				}
				return procList[i].RAM > procList[j].RAM
			})

			lt.cachedProcesses = procList
			lt.lastProcUpdate = time.Now()
		}
	}
	metrics.Processes = lt.cachedProcesses

	return metrics, nil
}

// LiveGPUMetrics holds real-time GPU stats (Exported)
type LiveGPUMetrics struct {
	HasGPU      bool
	GPUUsage    float64
	GPUMemUsage float64
	GPUMemUsed  uint64 // MB or MHz (Intel)
	GPUMemTotal uint64 // MB or MHz (Intel)
	GPUTemp     float64 // °C
}

func getGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	// 1. Try Nvidia-smi first
	if nvidia, err := getNvidiaGPUMetrics(); err == nil && nvidia.HasGPU {
		return nvidia, nil
	}

	if runtime.GOOS == "linux" {
		// 2. Try AMD GPU sysfs
		for _, card := range []string{"card0", "card1"} {
			busyPath := fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", card)
			if _, err := os.Stat(busyPath); err == nil {
				busyBytes, err := os.ReadFile(busyPath)
				if err == nil {
					busyVal, err := strconv.ParseFloat(strings.TrimSpace(string(busyBytes)), 64)
					if err == nil {
						metrics.HasGPU = true
						metrics.GPUUsage = busyVal

						// Try VRAM utilization
						memBusyPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_busy_percent", card)
						if memBytes, err := os.ReadFile(memBusyPath); err == nil {
							if memVal, err := strconv.ParseFloat(strings.TrimSpace(string(memBytes)), 64); err == nil {
								metrics.GPUMemUsage = memVal
							}
						}

						// Try VRAM Used / Total
						vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", card)
						vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", card)
						if uBytes, errU := os.ReadFile(vramUsedPath); errU == nil {
							if tBytes, errT := os.ReadFile(vramTotalPath); errT == nil {
								uVal, err1 := strconv.ParseUint(strings.TrimSpace(string(uBytes)), 10, 64)
								tVal, err2 := strconv.ParseUint(strings.TrimSpace(string(tBytes)), 10, 64)
								if err1 == nil && err2 == nil {
									metrics.GPUMemUsed = uVal / (1024 * 1024)
									metrics.GPUMemTotal = tVal / (1024 * 1024)
								}
							}
						}

						// Try GPU temp
						tempPath := fmt.Sprintf("/sys/class/drm/%s/device/hwmon/hwmon0/temp1_input", card)
						if _, errTemp := os.Stat(tempPath); errTemp != nil {
							tempPath = fmt.Sprintf("/sys/class/drm/%s/device/hwmon/hwmon1/temp1_input", card)
						}
						if tempBytes, errT := os.ReadFile(tempPath); errT == nil {
							tMilli, errParse := strconv.ParseFloat(strings.TrimSpace(string(tempBytes)), 64)
							if errParse == nil {
								metrics.GPUTemp = tMilli / 1000.0
							}
						}
						return metrics, nil
					}
				}
			}
		}

		// 3. Try Intel GPU sysfs (Frequency as utilization proxy)
		for _, card := range []string{"card0", "card1"} {
			actFreqPath := fmt.Sprintf("/sys/class/drm/%s/gt_act_freq_mhz", card)
			maxFreqPath := fmt.Sprintf("/sys/class/drm/%s/gt_max_freq_mhz", card)
			if _, err := os.Stat(actFreqPath); err == nil {
				actBytes, err1 := os.ReadFile(actFreqPath)
				maxBytes, err2 := os.ReadFile(maxFreqPath)
				if err1 == nil && err2 == nil {
					actVal, errAct := strconv.ParseFloat(strings.TrimSpace(string(actBytes)), 64)
					maxVal, errMax := strconv.ParseFloat(strings.TrimSpace(string(maxBytes)), 64)
					if errAct == nil && errMax == nil && maxVal > 0 {
						metrics.HasGPU = true
						metrics.GPUUsage = (actVal / maxVal) * 100.0
						metrics.GPUMemUsage = 0
						metrics.GPUMemUsed = uint64(actVal)
						metrics.GPUMemTotal = uint64(maxVal)
						metrics.GPUTemp = 0
						return metrics, nil
					}
				}
			}
		}
	}

	return metrics, fmt.Errorf("no GPU telemetry found")
}

func getNvidiaGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return metrics, err
	}

	cmd := exec.Command(path, "--query-gpu=utilization.gpu,utilization.memory,temperature.gpu,memory.used,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return metrics, err
	}

	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, ",")
	if len(parts) < 5 {
		return metrics, fmt.Errorf("invalid output format from nvidia-smi")
	}

	gpuUtil, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	memUtil, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	temp, err3 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	memUsed, err4 := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 64)
	memTotal, err5 := strconv.ParseUint(strings.TrimSpace(parts[4]), 10, 64)

	if err1 == nil && err2 == nil && err3 == nil && err4 == nil && err5 == nil {
		metrics.HasGPU = true
		metrics.GPUUsage = gpuUtil
		metrics.GPUMemUsage = memUtil
		metrics.GPUMemUsed = memUsed
		metrics.GPUMemTotal = memTotal
		metrics.GPUTemp = temp
		return metrics, nil
	}

	return metrics, fmt.Errorf("failed to parse GPU metrics")
}

