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

var (
	reShellVersion = regexp.MustCompile(`(\d+\.\d+(\.\d+)?)`)
	rePingWin      = regexp.MustCompile(`Average\s*=\s*(\d+)ms`)
	rePingUnix     = regexp.MustCompile(`(rtt|round-trip)\s+min/avg/max/(mdev|stddev)\s+=\s+([0-9.]+)/([0-9.]+)/([0-9.]+)/([0-9.]+)`)
	rePingUnixTime = regexp.MustCompile(`time=([0-9.]+)`)
)

// MockDistro is set at runtime to simulate other environments (Exported)
var MockDistro string

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

	if info.CPU == "" || info.CPU == "Unknown Processor" || strings.HasPrefix(info.CPU, "ARMv") || strings.HasPrefix(info.CPU, "AArch64") {
		cpuStats, err := cpu.Info()
		if err == nil && len(cpuStats) > 0 {
			if cpuStats[0].ModelName != "" {
				info.CPU = cpuStats[0].ModelName
			}
			mhz := cpuStats[0].Mhz
			if mhz > 1000 && info.CPUSpeed == "" {
				info.CPUSpeed = fmt.Sprintf("%.2f GHz", mhz/1000.0)
			} else if mhz > 0 && info.CPUSpeed == "" {
				info.CPUSpeed = fmt.Sprintf("%.0f MHz", mhz)
			}
		}
	}

	if info.CPU == "" {
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
	paths := []string{"/etc/os-release"}
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		paths = append(paths, prefix+"/etc/os-release")
	}

	var content []byte
	var err error
	for _, path := range paths {
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
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
			version = reShellVersion.FindString(firstLine)
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

func cleanGPUName(name string) string {
	name = strings.ReplaceAll(name, "Advanced Micro Devices, Inc. [AMD/ATI]", "AMD")
	name = strings.ReplaceAll(name, "Intel Corporation", "Intel")
	name = strings.ReplaceAll(name, "NVIDIA Corporation", "Nvidia")
	name = strings.ReplaceAll(name, "[AMD/ATI]", "AMD")
	fields := strings.Fields(name)
	return strings.Join(fields, " ")
}

func getLinuxGPU() string {
	cmd := exec.Command("lspci")
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

		var gpuInfo string
		if idx := strings.Index(lower, "vga compatible controller: "); idx != -1 {
			gpuInfo = line[idx+len("vga compatible controller: "):]
		} else if idx := strings.Index(lower, "3d controller: "); idx != -1 {
			gpuInfo = line[idx+len("3d controller: "):]
		} else if idx := strings.Index(lower, "display controller: "); idx != -1 {
			gpuInfo = line[idx+len("display controller: "):]
		} else {
			continue
		}

		gpuInfo = strings.TrimSpace(gpuInfo)
		if revIdx := strings.LastIndex(gpuInfo, " (rev "); revIdx != -1 {
			gpuInfo = strings.TrimSpace(gpuInfo[:revIdx])
		}

		gpuInfo = cleanGPUName(gpuInfo)
		if gpuInfo != "" && !seen[gpuInfo] {
			seen[gpuInfo] = true
			gpus = append(gpus, gpuInfo)
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

func getLinuxTerminal() string {
	ppid := os.Getppid()
	// Traverse up to 5 levels to find the terminal emulator
	for i := 0; i < 5; i++ {
		if ppid <= 1 {
			break
		}

		statPath := fmt.Sprintf("/proc/%d/stat", ppid)
		data, err := os.ReadFile(statPath)
		if err != nil {
			break
		}

		statStr := string(data)
		lastParen := strings.LastIndex(statStr, ")")
		if lastParen == -1 || lastParen+2 >= len(statStr) {
			break
		}

		firstParen := strings.Index(statStr, "(")
		var procName string
		if firstParen != -1 && firstParen < lastParen {
			procName = statStr[firstParen+1 : lastParen]
		}

		fields := strings.Fields(statStr[lastParen+2:])
		if len(fields) < 2 {
			break
		}

		parentPidStr := fields[1]
		parentPid := 0
		fmt.Sscanf(parentPidStr, "%d", &parentPid)

		lowerName := strings.ToLower(procName)
		switch lowerName {
		case "gnome-terminal-", "gnome-terminal":
			return "GNOME Terminal"
		case "konsole":
			return "Konsole"
		case "xfce4-terminal":
			return "XFCE Terminal"
		case "alacritty":
			return "Alacritty"
		case "kitty":
			return "Kitty"
		case "foot":
			return "Foot"
		case "urxvt", "rxvt-unicode", "urxvt-bin":
			return "urxvt"
		case "xterm":
			return "XTerm"
		case "st":
			return "st"
		case "wezterm-gui", "wezterm":
			return "WezTerm"
		case "tilix":
			return "Tilix"
		case "terminator":
			return "Terminator"
		case "guake":
			return "Guake"
		case "tilda":
			return "Tilda"
		case "yakuake":
			return "Yakuake"
		case "lxterminal":
			return "LXTerminal"
		case "cool-retro-ter":
			return "Cool Retro Term"
		case "tmux: client", "tmux", "screen":
			// Multiplexers, keep going up to find the terminal emulator
		case "bash", "zsh", "sh", "fish", "dash", "tcsh", "csh", "ksh":
			// Shells, keep going up
		case "sudo", "su":
			// Privilege escalations, keep going up
		default:
			if strings.HasSuffix(lowerName, "terminal") || strings.HasSuffix(lowerName, "term") || strings.Contains(lowerName, "terminal-server") {
				cleanName := strings.TrimSuffix(lowerName, "-server")
				cleanName = strings.TrimSuffix(cleanName, "-gui")
				return strings.Title(cleanName)
			}
		}

		if parentPid <= 0 || parentPid == ppid {
			break
		}
		ppid = parentPid
	}
	return ""
}

func getTerminal() string {
	// 1. Check TERM_PROGRAM (common on macOS, VS Code, Warp, etc.)
	termProg := os.Getenv("TERM_PROGRAM")
	if termProg != "" {
		termProg = strings.TrimSuffix(termProg, ".app")
		termProg = strings.Replace(termProg, "iTerm", "iTerm2", 1)
		return strings.Title(termProg)
	}

	// 2. Check specific env variables set by terminal emulators
	if os.Getenv("ALACRITTY_LOG") != "" || os.Getenv("ALACRITTY_SOCKET") != "" {
		return "Alacritty"
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("KITTY_PID") != "" {
		return "Kitty"
	}
	if os.Getenv("WT_SESSION") != "" || os.Getenv("WT_PROFILE_ID") != "" {
		return "Windows Terminal"
	}
	if os.Getenv("KONSOLE_VERSION") != "" || os.Getenv("KONSOLE_PROFILE_NAME") != "" {
		return "Konsole"
	}

	// 4. Linux specific parent process resolution
	if runtime.GOOS == "linux" {
		if term := getLinuxTerminal(); term != "" {
			return term
		}
	}

	// 5. Fallback to TERM if it's not a generic name
	term := os.Getenv("TERM")
	if term != "" && term != "xterm-256color" && term != "screen" && term != "linux" && term != "xterm" {
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
		matches := rePingWin.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			return fmt.Sprintf("%s ms", matches[1])
		}
	} else {
		matches := rePingUnix.FindStringSubmatch(outputStr)
		if len(matches) > 4 {
			return fmt.Sprintf("%s ms", matches[4])
		}
		matches2 := rePingUnixTime.FindStringSubmatch(outputStr)
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
	if MockDistro != "" {
		return GetMockSystemInfo(MockDistro)
	}

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
	if MockDistro != "" {
		return GetMockProcessList(), nil
	}

	allProcs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to get processes: %w", err)
	}

	var wg sync.WaitGroup
	results := make(chan ProcessInfo, len(allProcs))
	sem := make(chan struct{}, 32) // Limit concurrency to avoid FD exhaustion

	// Step 1: Prime the CPU percent calculation by calling it once on all processes.
	for _, p := range allProcs {
		if p == nil {
			continue
		}
		wg.Add(1)
		go func(proc *process.Process) {
			defer wg.Done()
			defer func() { _ = recover() }()
			if proc == nil {
				return
			}
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
		if p == nil {
			continue
		}
		wg.Add(1)
		go func(proc *process.Process) {
			defer wg.Done()
			defer func() { _ = recover() }()
			if proc == nil {
				return
			}
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
	if MockDistro != "" {
		return GetMockNetworkDetails(), nil
	}

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
	procsMap    map[int32]*process.Process
	prevNetRx   uint64
	prevNetTx   uint64
	prevNetTime time.Time
	mu          sync.Mutex
	bootTime    time.Time

	// Cached processes list
	cachedProcesses []ProcessInfo
	lastProcUpdate  time.Time
	procScanning    bool

	// Cached disk usage
	cachedDiskUsed    uint64
	cachedDiskTotal   uint64
	cachedDiskPercent float64
	lastDiskUpdate    time.Time

	// Cached temperature
	cachedTemp     float64
	lastTempUpdate time.Time

	// Cached net interface name
	cachedNetIface  string
	lastIfaceUpdate time.Time

	// Cached GPU metrics
	cachedGPUMetrics LiveGPUMetrics
	lastGPUUpdate    time.Time

	// Smoothed metrics for visual stability (anti-jitter)
	smoothedCPU   float64
	smoothedCores []float64
	smoothedNetRx float64
	smoothedNetTx float64
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
	if MockDistro != "" {
		return GetMockLiveMetrics(MockDistro), nil
	}

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

	// 2. CPU Usage & Per-Core Percentages (real-time, queried with EMA anti-jitter smoothing)
	percentages, err := cpu.Percent(0, true)
	if err == nil && len(percentages) > 0 {
		var sum float64
		for _, p := range percentages {
			sum += p
		}
		rawOverall := sum / float64(len(percentages))

		// Apply Exponential Moving Average for smooth, non-jittery meter display
		if lt.smoothedCPU == 0 {
			lt.smoothedCPU = rawOverall
		} else {
			lt.smoothedCPU = 0.40*rawOverall + 0.60*lt.smoothedCPU
		}
		metrics.CPUUsage = lt.smoothedCPU

		if len(lt.smoothedCores) != len(percentages) {
			lt.smoothedCores = make([]float64, len(percentages))
			copy(lt.smoothedCores, percentages)
		} else {
			for i := range percentages {
				lt.smoothedCores[i] = 0.40*percentages[i] + 0.60*lt.smoothedCores[i]
			}
		}
		metrics.CPUCores = make([]float64, len(lt.smoothedCores))
		copy(metrics.CPUCores, lt.smoothedCores)
	} else {
		overall, err := cpu.Percent(0, false)
		if err == nil && len(overall) > 0 {
			if lt.smoothedCPU == 0 {
				lt.smoothedCPU = overall[0]
			} else {
				lt.smoothedCPU = 0.40*overall[0] + 0.60*lt.smoothedCPU
			}
			metrics.CPUUsage = lt.smoothedCPU
		}
	}

	// 3. Memory (real-time)
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err == nil {
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
						memMap[key] = val * 1024
					}
				}
			}

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

	// 6. Network Rates (Iface list cached for 10 seconds, stats queried in real-time)
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
			rawRxSpeed := 0.0
			rawTxSpeed := 0.0
			if rxTotal >= lt.prevNetRx {
				rawRxSpeed = float64(rxTotal-lt.prevNetRx) / elapsed
			}
			if txTotal >= lt.prevNetTx {
				rawTxSpeed = float64(txTotal-lt.prevNetTx) / elapsed
			}

			if lt.smoothedNetRx == 0 {
				lt.smoothedNetRx = rawRxSpeed
			} else {
				lt.smoothedNetRx = 0.50*rawRxSpeed + 0.50*lt.smoothedNetRx
			}

			if lt.smoothedNetTx == 0 {
				lt.smoothedNetTx = rawTxSpeed
			} else {
				lt.smoothedNetTx = 0.50*rawTxSpeed + 0.50*lt.smoothedNetTx
			}
			metrics.NetRxSpeed = lt.smoothedNetRx
			metrics.NetTxSpeed = lt.smoothedNetTx
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

	// 8. Processes (asynchronous background scanning for zero UI delay)
	if (time.Since(lt.lastProcUpdate) >= 1500*time.Millisecond || len(lt.cachedProcesses) == 0) && !lt.procScanning {
		lt.procScanning = true
		go lt.scanProcessesAsync()
	}
	metrics.Processes = lt.cachedProcesses

	return metrics, nil
}

func (lt *LiveTracker) scanProcessesAsync() {
	defer func() {
		lt.mu.Lock()
		lt.procScanning = false
		lt.mu.Unlock()
	}()

	pids, err := process.Pids()
	if err != nil {
		return
	}
	currentPids := make(map[int32]bool, len(pids))
	for _, pid := range pids {
		currentPids[pid] = true
	}

	lt.mu.Lock()
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
	procs := make([]*process.Process, 0, len(lt.procsMap))
	for _, p := range lt.procsMap {
		if p != nil {
			procs = append(procs, p)
		}
	}
	lt.mu.Unlock()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	results := make(chan ProcessInfo, len(procs))

	for _, proc := range procs {
		wg.Add(1)
		go func(p *process.Process) {
			defer wg.Done()
			defer func() { _ = recover() }()
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

	lt.mu.Lock()
	lt.cachedProcesses = procList
	lt.lastProcUpdate = time.Now()
	lt.mu.Unlock()
}

// LiveGPUMetrics holds real-time GPU stats (Exported)
type LiveGPUMetrics struct {
	HasGPU      bool
	GPUUsage    float64
	GPUMemUsage float64
	GPUMemUsed  uint64  // MB or MHz (Intel)
	GPUMemTotal uint64  // MB or MHz (Intel)
	GPUTemp     float64 // °C
}

func getAMDOrIntelTemp(cardName string) float64 {
	hwmonDir := fmt.Sprintf("/sys/class/drm/%s/device/hwmon", cardName)
	files, err := os.ReadDir(hwmonDir)
	if err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "hwmon") {
				tempPath := fmt.Sprintf("%s/%s/temp1_input", hwmonDir, f.Name())
				if bytes, err := os.ReadFile(tempPath); err == nil {
					if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(bytes)), 64); err == nil {
						return mVal / 1000.0
					}
				}
			}
		}
	}
	return 0
}

func getGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	// 1. Try Nvidia-smi first (Proprietary Nvidia driver)
	if nvidia, err := getNvidiaGPUMetrics(); err == nil && nvidia.HasGPU {
		return nvidia, nil
	}

	if runtime.GOOS == "linux" {
		// Scan all active DRM cards
		files, err := os.ReadDir("/sys/class/drm")
		if err == nil {
			for _, file := range files {
				cardName := file.Name()
				if !strings.HasPrefix(cardName, "card") || strings.Contains(cardName, "-") {
					continue
				}

				devicePath := fmt.Sprintf("/sys/class/drm/%s/device", cardName)
				if _, err := os.Stat(devicePath); err != nil {
					continue
				}

				// Read Vendor ID
				vendorBytes, err := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/device/vendor", cardName))
				if err != nil {
					continue
				}
				vendorID := strings.TrimSpace(strings.ToLower(string(vendorBytes)))

				// 2. AMD GPU (Vendor ID: 0x1002)
				if strings.Contains(vendorID, "1002") {
					metrics.HasGPU = true

					// Try reading GPU busy/usage
					busyPath := fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", cardName)
					if busyBytes, err := os.ReadFile(busyPath); err == nil {
						if busyVal, err := strconv.ParseFloat(strings.TrimSpace(string(busyBytes)), 64); err == nil {
							metrics.GPUUsage = busyVal
						}
					}

					// Try reading VRAM stats
					vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", cardName)
					vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", cardName)
					if uBytes, errU := os.ReadFile(vramUsedPath); errU == nil {
						if tBytes, errT := os.ReadFile(vramTotalPath); errT == nil {
							uVal, err1 := strconv.ParseUint(strings.TrimSpace(string(uBytes)), 10, 64)
							tVal, err2 := strconv.ParseUint(strings.TrimSpace(string(tBytes)), 10, 64)
							if err1 == nil && err2 == nil && tVal > 0 {
								metrics.GPUMemUsed = uVal / (1024 * 1024)
								metrics.GPUMemTotal = tVal / (1024 * 1024)
								metrics.GPUMemUsage = (float64(uVal) / float64(tVal)) * 100.0
							}
						}
					}

					// Try reading temperature from hwmon
					metrics.GPUTemp = getAMDOrIntelTemp(cardName)
					return metrics, nil
				}

				// 3. Intel GPU (Vendor ID: 0x8086)
				if strings.Contains(vendorID, "8086") {
					actFreqPath := fmt.Sprintf("/sys/class/drm/%s/gt_act_freq_mhz", cardName)
					maxFreqPath := fmt.Sprintf("/sys/class/drm/%s/gt_max_freq_mhz", cardName)
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
								metrics.GPUTemp = getAMDOrIntelTemp(cardName)
								return metrics, nil
							}
						}
					}
				}

				// 4. Nvidia GPU (Vendor ID: 0x10de) (Nouveau/open-source fallback when nvidia-smi is missing)
				if strings.Contains(vendorID, "10de") {
					metrics.HasGPU = true

					// Try reading GPU busy/usage
					busyPath := fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", cardName)
					if busyBytes, err := os.ReadFile(busyPath); err == nil {
						if busyVal, err := strconv.ParseFloat(strings.TrimSpace(string(busyBytes)), 64); err == nil {
							metrics.GPUUsage = busyVal
						}
					}

					// Try reading VRAM stats
					vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", cardName)
					vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", cardName)
					if uBytes, errU := os.ReadFile(vramUsedPath); errU == nil {
						if tBytes, errT := os.ReadFile(vramTotalPath); errT == nil {
							uVal, err1 := strconv.ParseUint(strings.TrimSpace(string(uBytes)), 10, 64)
							tVal, err2 := strconv.ParseUint(strings.TrimSpace(string(tBytes)), 10, 64)
							if err1 == nil && err2 == nil && tVal > 0 {
								metrics.GPUMemUsed = uVal / (1024 * 1024)
								metrics.GPUMemTotal = tVal / (1024 * 1024)
								metrics.GPUMemUsage = (float64(uVal) / float64(tVal)) * 100.0
							}
						}
					}

					metrics.GPUTemp = getAMDOrIntelTemp(cardName)
					return metrics, nil
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

// GPUDetails holds comprehensive details about the graphics hardware and libraries
type GPUDetails struct {
	Name        string
	Vendor      string
	Driver      string
	VRAMUsed    string
	VRAMTotal   string
	VRAMFree    string
	Temperature string
	OpenGL      string
	Vulkan      string
	OpenCL      string
	CUDA        string
}

// GetGPUDetails collects detailed GPU and graphics API information
func GetGPUDetails() *GPUDetails {
	if MockDistro != "" {
		return GetMockGPUDetails(MockDistro)
	}

	details := &GPUDetails{
		Name:        "Unknown GPU",
		Vendor:      "Unknown",
		Driver:      "Unknown",
		VRAMUsed:    "Unknown",
		VRAMTotal:   "Unknown",
		VRAMFree:    "Unknown",
		Temperature: "Unknown",
		OpenGL:      "Unknown",
		Vulkan:      "Unknown",
		OpenCL:      "Unknown",
		CUDA:        "Unknown",
	}

	if details.Name == "" || details.Name == "Unknown GPU" {
		if name := getLinuxGPU(); name != "" {
			details.Name = name
		} else if name := getGPUInfo(); name != "" && name != "Unknown" {
			details.Name = name
		}
	}

	// 2. Resolve Graphics Libraries
	details.OpenGL = getOpenGLVersion()
	details.Vulkan = getVulkanVersion()
	details.OpenCL = getOpenCLVersion()
	details.CUDA = getCUDAVersion()

	// 3. Resolve Vendor, Driver, VRAM, and Temp based on OS
	if runtime.GOOS == "linux" {
		// Scan active DRM cards
		files, err := os.ReadDir("/sys/class/drm")
		if err == nil {
			for _, file := range files {
				cardName := file.Name()
				if !strings.HasPrefix(cardName, "card") || strings.Contains(cardName, "-") {
					continue
				}
				devicePath := fmt.Sprintf("/sys/class/drm/%s/device", cardName)
				if _, err := os.Stat(devicePath); err != nil {
					continue
				}

				// Resolve Driver
				driverLink := fmt.Sprintf("/sys/class/drm/%s/device/driver", cardName)
				if target, err := os.Readlink(driverLink); err == nil {
					parts := strings.Split(target, "/")
					details.Driver = parts[len(parts)-1]
				}

				// Resolve Vendor
				vendorBytes, err := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/device/vendor", cardName))
				if err == nil {
					vendorID := strings.TrimSpace(strings.ToLower(string(vendorBytes)))
					if strings.Contains(vendorID, "1002") {
						details.Vendor = "AMD"
					} else if strings.Contains(vendorID, "8086") {
						details.Vendor = "Intel"
					} else if strings.Contains(vendorID, "10de") {
						details.Vendor = "Nvidia"
					}
				}

				// Resolve VRAM
				vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", cardName)
				vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", cardName)
				if uBytes, errU := os.ReadFile(vramUsedPath); errU == nil {
					if tBytes, errT := os.ReadFile(vramTotalPath); errT == nil {
						uVal, err1 := strconv.ParseUint(strings.TrimSpace(string(uBytes)), 10, 64)
						tVal, err2 := strconv.ParseUint(strings.TrimSpace(string(tBytes)), 10, 64)
						if err1 == nil && err2 == nil && tVal > 0 {
							details.VRAMUsed = FormatBytes(uVal)
							details.VRAMTotal = FormatBytes(tVal)
							if tVal >= uVal {
								details.VRAMFree = FormatBytes(tVal - uVal)
							}
						}
					}
				}

				// Resolve Temp
				if temp := getAMDOrIntelTemp(cardName); temp > 0 {
					details.Temperature = fmt.Sprintf("%.1f °C", temp)
				}
				break // Stop after finding the first active card
			}
		}

		// Fallback/Override if nvidia-smi is available
		if nvidia, err := getNvidiaGPUMetrics(); err == nil && nvidia.HasGPU {
			details.Vendor = "Nvidia"
			details.VRAMUsed = fmt.Sprintf("%d MB", nvidia.GPUMemUsed)
			details.VRAMTotal = fmt.Sprintf("%d MB", nvidia.GPUMemTotal)
			if nvidia.GPUMemTotal >= nvidia.GPUMemUsed {
				details.VRAMFree = fmt.Sprintf("%d MB", nvidia.GPUMemTotal-nvidia.GPUMemUsed)
			}
			details.Temperature = fmt.Sprintf("%.1f °C", nvidia.GPUTemp)
			// Read driver from nvidia-smi
			cmd := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
			if out, err := cmd.Output(); err == nil {
				details.Driver = "Nvidia proprietary (" + strings.TrimSpace(string(out)) + ")"
			}
		}
	} else if runtime.GOOS == "windows" {
		// Try using wmic
		cmd := exec.Command("wmic", "path", "win32_VideoController", "get", "AdapterRAM,DriverVersion,VideoProcessor")
		if out, err := cmd.Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			if len(lines) > 1 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 3 {
					details.Driver = fields[1]
					if ramBytes, err := strconv.ParseUint(fields[0], 10, 64); err == nil && ramBytes > 0 {
						details.VRAMTotal = FormatBytes(ramBytes)
					}
				}
			}
		}
		lowerName := strings.ToLower(details.Name)
		if strings.Contains(lowerName, "nvidia") {
			details.Vendor = "Nvidia"
			if nvidia, err := getNvidiaGPUMetrics(); err == nil && nvidia.HasGPU {
				details.VRAMUsed = fmt.Sprintf("%d MB", nvidia.GPUMemUsed)
				details.VRAMTotal = fmt.Sprintf("%d MB", nvidia.GPUMemTotal)
				if nvidia.GPUMemTotal >= nvidia.GPUMemUsed {
					details.VRAMFree = fmt.Sprintf("%d MB", nvidia.GPUMemTotal-nvidia.GPUMemUsed)
				}
				details.Temperature = fmt.Sprintf("%.1f °C", nvidia.GPUTemp)
			}
		} else if strings.Contains(lowerName, "amd") || strings.Contains(lowerName, "radeon") {
			details.Vendor = "AMD"
		} else if strings.Contains(lowerName, "intel") {
			details.Vendor = "Intel"
		}
	} else if runtime.GOOS == "darwin" {
		// macOS
		cmd := exec.Command("system_profiler", "SPDisplaysDataType")
		if out, err := cmd.Output(); err == nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				lineLower := strings.ToLower(line)
				if strings.Contains(lineLower, "vendor:") {
					details.Vendor = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				} else if strings.Contains(lineLower, "vram (total):") {
					details.VRAMTotal = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				} else if strings.Contains(lineLower, "metal:") {
					details.Vulkan = "Metal (" + strings.TrimSpace(strings.SplitN(line, ":", 2)[1]) + ")"
				}
			}
		}
	}

	return details
}

func getOpenGLVersion() string {
	cmd := exec.Command("glxinfo")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "OpenGL version string:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "OpenGL version string:"))
			}
		}
	}
	cmd = exec.Command("eglinfo")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "EGL version string:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "EGL version string:"))
			}
		}
	}
	return "Unknown"
}

func getVulkanVersion() string {
	cmd := exec.Command("vulkaninfo", "--summary")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Vulkan Instance Version:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Vulkan Instance Version:"))
			}
		}
	}
	cmd = exec.Command("vulkaninfo")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Vulkan Instance Version") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return "Unknown"
}

func getOpenCLVersion() string {
	cmd := exec.Command("clinfo")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Platform Version") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return "Unknown"
}

func getCUDAVersion() string {
	cmd := exec.Command("nvcc", "--version")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "release") {
				idx := strings.Index(line, "release")
				return strings.TrimSpace(line[idx:])
			}
		}
	}
	cmd = exec.Command("nvidia-smi")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "CUDA Version:") {
				idx := strings.Index(line, "CUDA Version:")
				part := line[idx:]
				part = strings.TrimSuffix(part, "|")
				return strings.TrimSpace(strings.TrimPrefix(part, "CUDA Version:"))
			}
		}
	}
	return "Unknown"
}

// GetMockSystemInfo returns mock system specifications
func GetMockSystemInfo(distro string) *SystemInfo {
	info := &SystemInfo{
		Hostname:      "mock-pc",
		Uptime:        "2 hours, 15 mins",
		Shell:         "zsh 5.9",
		Resolution:    "1920x1080",
		DE:            "GNOME",
		WindowManager: "Mutter",
		Terminal:      "Alacritty",
		Locale:        "en_US.UTF-8",
		Languages:     "Go, Rust, TypeScript",
		OpenPorts:     "22, 80, 443, 3000",
		Temperature:   "45.0 °C",
		Go:            "go1.21.0",
		Packages:      "Pacman (1200)",
	}

	switch distro {
	case "arch":
		info.OS = "Arch Linux x86_64"
		info.Kernel = "Linux 6.10.1-arch1-1"
		info.CPU = "AMD Ryzen 7 7800X3D @ 4.20 GHz"
		info.CoresThreads = "8/16"
		info.CPUSpeed = "4.20 GHz"
		info.GPU = "Nvidia GeForce RTX 4090"
		info.RAM = "8.5GB / 32.0GB (26%)"
		info.Swap = "2.0GB / 8.0GB (25%)"
		info.Disk = "150GB / 1000GB (15%)"
		info.IPAddress = "192.168.1.100"
	case "ubuntu":
		info.OS = "Ubuntu 24.04 LTS"
		info.Kernel = "Linux 6.8.0-31-generic"
		info.CPU = "Intel(R) Core(TM) i7-1370P @ 2.20 GHz"
		info.CoresThreads = "14/20"
		info.CPUSpeed = "5.20 GHz"
		info.GPU = "Intel Iris Xe Graphics"
		info.RAM = "4.1GB / 16.0GB (25%)"
		info.Swap = "1.0GB / 4.0GB (25%)"
		info.Disk = "64GB / 512GB (12%)"
		info.IPAddress = "192.168.1.105"
		info.Packages = "Dpkg (1850), Snap (12)"
		info.Shell = "bash 5.2"
		info.Terminal = "GNOME Terminal"
	case "macos":
		info.OS = "macOS Sonoma 14.5"
		info.Kernel = "Darwin 23.5.0"
		info.CPU = "Apple M3 Max"
		info.CoresThreads = "16/16"
		info.CPUSpeed = "3.40 GHz"
		info.GPU = "Apple M3 Max 30-Core GPU"
		info.RAM = "12.3GB / 48.0GB (25%)"
		info.Swap = "0.0GB / 0.0GB (0%)"
		info.Disk = "256GB / 1000GB (25%)"
		info.IPAddress = "10.0.0.15"
		info.Packages = "Homebrew (120)"
		info.Terminal = "Terminal.app"
		info.DE = "Aqua"
		info.WindowManager = "Quartz Compositor"
	case "windows":
		info.OS = "Microsoft Windows 11 Pro"
		info.Kernel = "NT 10.0.22631"
		info.CPU = "Intel(R) Core(TM) i9-14900HX @ 2.20 GHz"
		info.CoresThreads = "24/32"
		info.CPUSpeed = "5.80 GHz"
		info.GPU = "Nvidia GeForce RTX 4080 Laptop GPU"
		info.RAM = "14.2GB / 64.0GB (22%)"
		info.Swap = "4.0GB / 16.0GB (25%)"
		info.Disk = "450GB / 2000GB (22%)"
		info.IPAddress = "192.168.1.20"
		info.Packages = "Winget (45)"
		info.Shell = "PowerShell 7.4.2"
		info.Terminal = "Windows Terminal"
		info.DE = "Explorer"
		info.WindowManager = "DWM"
	default:
		info.OS = "Generic Linux"
		info.Kernel = "Linux 6.1.0"
		info.CPU = "Generic Processor"
		info.CoresThreads = "4/8"
		info.CPUSpeed = "3.00 GHz"
		info.GPU = "Generic Graphics"
		info.RAM = "2.0GB / 8.0GB (25%)"
		info.Swap = "0.0GB / 0.0GB (0%)"
		info.Disk = "20GB / 100GB (20%)"
		info.IPAddress = "127.0.0.1"
	}
	return info
}

// GetMockGPUDetails returns mock GPU and API specifications
func GetMockGPUDetails(distro string) *GPUDetails {
	details := &GPUDetails{
		Name:        "Generic GPU",
		Vendor:      "Generic",
		Driver:      "generic-driver",
		VRAMUsed:    "512 MB",
		VRAMTotal:   "2.0 GB",
		VRAMFree:    "1.5 GB",
		Temperature: "40.0 °C",
		OpenGL:      "OpenGL 4.5",
		Vulkan:      "Vulkan 1.2",
		OpenCL:      "OpenCL 2.0",
		CUDA:        "Unknown",
	}

	switch distro {
	case "arch":
		details.Name = "Nvidia GeForce RTX 4090"
		details.Vendor = "Nvidia"
		details.Driver = "Nvidia proprietary (555.58)"
		details.VRAMUsed = "6.1 GB"
		details.VRAMTotal = "24.0 GB"
		details.VRAMFree = "17.9 GB"
		details.Temperature = "52.0 °C"
		details.OpenGL = "OpenGL 4.6 (Compatibility Profile) NVIDIA 555.58"
		details.Vulkan = "Vulkan 1.3.277"
		details.OpenCL = "OpenCL 3.0 CUDA"
		details.CUDA = "release 12.5, V12.5.82"
	case "ubuntu":
		details.Name = "Intel Iris Xe Graphics"
		details.Vendor = "Intel"
		details.Driver = "i915"
		details.VRAMUsed = "1.2 GB"
		details.VRAMTotal = "8.0 GB"
		details.VRAMFree = "6.8 GB"
		details.Temperature = "46.0 °C"
		details.OpenGL = "OpenGL 4.6 Mesa 24.0.5"
		details.Vulkan = "Vulkan 1.3.274"
		details.OpenCL = "OpenCL 3.0 Neo"
	case "macos":
		details.Name = "Apple M3 Max GPU"
		details.Vendor = "Apple"
		details.Driver = "Apple Metal Driver"
		details.VRAMTotal = "48.0 GB"
		details.Vulkan = "Metal (310.25)"
	case "windows":
		details.Name = "Nvidia GeForce RTX 4080 Laptop GPU"
		details.Vendor = "Nvidia"
		details.Driver = "Nvidia Game Ready (552.44)"
		details.VRAMUsed = "3.2 GB"
		details.VRAMTotal = "12.0 GB"
		details.VRAMFree = "8.8 GB"
		details.Temperature = "62.0 °C"
		details.OpenGL = "OpenGL 4.6 NVIDIA 552.44"
		details.Vulkan = "Vulkan 1.3.278"
		details.OpenCL = "OpenCL 3.0 CUDA"
		details.CUDA = "release 12.4"
	}
	return details
}

// GetMockProcessList returns a mock list of processes
func GetMockProcessList() []ProcessInfo {
	return []ProcessInfo{
		{PID: 1042, Name: "kernelview", CPU: 0.8, RAM: 45 * 1024 * 1024},
		{PID: 3012, Name: "firefox", CPU: 12.5, RAM: 820 * 1024 * 1024},
		{PID: 4511, Name: "vscode", CPU: 4.2, RAM: 450 * 1024 * 1024},
		{PID: 1205, Name: "discord", CPU: 2.1, RAM: 180 * 1024 * 1024},
		{PID: 902, Name: "systemd", CPU: 0.1, RAM: 15 * 1024 * 1024},
		{PID: 1512, Name: "kitty", CPU: 1.5, RAM: 60 * 1024 * 1024},
		{PID: 2804, Name: "postgres", CPU: 0.3, RAM: 120 * 1024 * 1024},
	}
}

// GetMockNetworkDetails returns mock network statistics
func GetMockNetworkDetails() *NetworkInfo {
	return &NetworkInfo{
		Hostname:   "mock-pc",
		PrivateIP:  "192.168.1.100",
		MACAddress: "00:11:22:33:44:55",
		PublicIP:   "203.0.113.50",
		ISP:        "Mock Telecom",
		City:       "New York",
		Country:    "United States",
		DNSServers: []string{"1.1.1.1", "8.8.8.8"},
		Ping:       "14.5 ms",
		IOCounters: "Rx: 45.0 GB / Tx: 5.0 GB",
	}
}

// GetMockLiveMetrics returns real-time live TUI metrics for the mock distro
func GetMockLiveMetrics(distro string) *LiveMetrics {
	sysInfo := GetMockSystemInfo(distro)

	var numCores int
	switch distro {
	case "arch":
		numCores = 16
	case "ubuntu":
		numCores = 20
	case "macos":
		numCores = 16
	case "windows":
		numCores = 32
	default:
		numCores = 8
	}

	cpuCores := make([]float64, numCores)
	for i := range cpuCores {
		cpuCores[i] = 5.0 + float64(time.Now().UnixNano()%40)
	}

	var usedMem, totalMem uint64
	switch distro {
	case "arch":
		usedMem = 8500 * 1024 * 1024
		totalMem = 32000 * 1024 * 1024
	case "ubuntu":
		usedMem = 4100 * 1024 * 1024
		totalMem = 16000 * 1024 * 1024
	case "macos":
		usedMem = 12300 * 1024 * 1024
		totalMem = 48000 * 1024 * 1024
	case "windows":
		usedMem = 14200 * 1024 * 1024
		totalMem = 64000 * 1024 * 1024
	default:
		usedMem = 2000 * 1024 * 1024
		totalMem = 8000 * 1024 * 1024
	}

	var temp float64
	fmt.Sscanf(sysInfo.Temperature, "%f", &temp)

	gpuDetails := GetMockGPUDetails(distro)
	var gpuTemp float64
	fmt.Sscanf(gpuDetails.Temperature, "%f", &gpuTemp)

	var gpuMemUsed, gpuMemTotal uint64
	fmt.Sscanf(gpuDetails.VRAMUsed, "%d", &gpuMemUsed)
	fmt.Sscanf(gpuDetails.VRAMTotal, "%d", &gpuMemTotal)

	gpuMetrics := LiveGPUMetrics{
		HasGPU:      gpuDetails.Vendor != "Unknown" && gpuDetails.Vendor != "Generic",
		GPUUsage:    10.0 + float64(time.Now().UnixNano()%30),
		GPUMemUsage: 25.0 + float64(time.Now().UnixNano()%10),
		GPUMemUsed:  gpuMemUsed,
		GPUMemTotal: gpuMemTotal,
		GPUTemp:     gpuTemp,
	}

	return &LiveMetrics{
		Uptime:      sysInfo.Uptime,
		CPUUsage:    20.0 + float64(time.Now().UnixNano()%15),
		CPUCores:    cpuCores,
		RAMUsed:     usedMem,
		RAMTotal:    totalMem,
		RAMPercent:  (float64(usedMem) / float64(totalMem)) * 100.0,
		SwapUsed:    1000 * 1024 * 1024,
		SwapTotal:   4000 * 1024 * 1024,
		SwapPercent: 25.0,
		DiskUsed:    150000 * 1024 * 1024,
		DiskTotal:   1000000 * 1024 * 1024,
		DiskPercent: 15.0,
		Temperature: temp,
		NetIface:    "eth0",
		NetRxSpeed:  12500000,
		NetTxSpeed:  1500000,
		NetRxTotal:  45000000000,
		NetTxTotal:  5000000000,
		Processes:   GetMockProcessList(),
		GPUMetrics:  gpuMetrics,
	}
}
