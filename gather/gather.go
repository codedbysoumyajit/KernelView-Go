package gather

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

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
	reValidHost    = regexp.MustCompile(`^[a-zA-Z0-9.\-_:]+$`)
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

// NetworkInterfaceDetail holds individual interface telemetry (Exported)
type NetworkInterfaceDetail struct {
	Name      string
	Type      string // "Wi-Fi", "Ethernet", "Loopback", "Virtual"
	State     string // "UP", "DOWN"
	IPv4      string
	IPv6      string
	MAC       string
	MTU       int
	Speed     string
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
	RxErrors  uint64
	TxErrors  uint64
	RxDropped uint64
	TxDropped uint64
}

// WifiInfo holds wireless-specific telemetry (Exported)
type WifiInfo struct {
	SSID       string
	BSSID      string
	SignalDBm  int
	SignalPerc int
	Freq       string
	Channel    string
	Bitrate    string
	Security   string
}

// NetworkInfo holds detailed network data (Exported)
type NetworkInfo struct {
	Hostname     string
	PrimaryIface string
	IfaceType    string
	PrivateIP    string
	IPv6Address  string
	MACAddress   string
	Gateway      string
	DNSServers   []string
	PublicIP     string
	ISP          string
	City         string
	Country      string
	Proxy        string
	Ping         string // e.g., "15.2 ms"
	IOCounters   string // Network I/O
	RxTotal      string
	TxTotal      string
	PacketStats  string
	SocketStats  string
	Wifi         *WifiInfo
	Interfaces   []NetworkInterfaceDetail
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
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		for j, r := range runes {
			if unicode.IsLetter(r) {
				runes[j] = unicode.ToUpper(r)
				break
			}
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// FormatBytes formats a byte count into a human-readable string (Exported)
func FormatBytes(b uint64) string {
	if b >= (1 << 30) {
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	}
	if b >= (1 << 20) {
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	}
	if b >= (1 << 10) {
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

func getWindowsRegValue(key, valName string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	out := runCommand("reg", "query", key, "/v", valName)
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(valName)) {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return strings.Join(fields[2:], " ")
			}
		}
	}
	return ""
}

func extractPlistString(data []byte, key string) string {
	keyTag := "<key>" + key + "</key>"
	idx := bytes.Index(data, []byte(keyTag))
	if idx == -1 {
		return ""
	}
	sub := data[idx+len(keyTag):]
	startTag := []byte("<string>")
	endTag := []byte("</string>")
	sIdx := bytes.Index(sub, startTag)
	if sIdx == -1 {
		return ""
	}
	eIdx := bytes.Index(sub[sIdx+len(startTag):], endTag)
	if eIdx == -1 {
		return ""
	}
	return strings.TrimSpace(string(sub[sIdx+len(startTag) : sIdx+len(startTag)+eIdx]))
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
		if model := runCommand("sysctl", "-n", "hw.model"); model != "" {
			return model
		}
	case "windows":
		if model := getWindowsRegValue(`HKLM\HARDWARE\DESCRIPTION\System\BIOS`, "SystemProductName"); model != "" && model != "System Product Name" {
			return model
		}
		if model := runCommand("wmic", "computersystem", "get", "model"); model != "" {
			lines := strings.Split(model, "\n")
			if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
				return strings.TrimSpace(lines[1])
			}
		}
	case "freebsd", "openbsd", "netbsd":
		if model := runCommand("sysctl", "-n", "hw.model"); model != "" {
			return model
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
	info.Kernel = fmt.Sprintf("%s %s", titleCase(kernelName), h.KernelVersion)
	info.Hostname, _ = os.Hostname()
}

func getLinuxCPUInfo() (model string, speed string, coresThreads string) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", "", ""
	}
	lines := strings.Split(string(data), "\n")
	var cpuModel string
	var mhz float64
	processors := 0
	coreIDs := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Processor") || strings.HasPrefix(line, "Hardware") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && cpuModel == "" {
				cpuModel = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "cpu MHz") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && mhz == 0 {
				mhz, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			}
		}
		if strings.HasPrefix(line, "processor") {
			processors++
		}
		if strings.HasPrefix(line, "core id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				coreIDs[strings.TrimSpace(parts[1])] = true
			}
		}
	}

	if mhz > 1000 {
		speed = fmt.Sprintf("%.2f GHz", mhz/1000.0)
	} else if mhz > 0 {
		speed = fmt.Sprintf("%.0f MHz", mhz)
	}

	physicalCores := len(coreIDs)
	if physicalCores == 0 {
		physicalCores = processors
	}
	if processors > 0 {
		coresThreads = fmt.Sprintf("%d/%d", physicalCores, processors)
	}

	return cpuModel, speed, coresThreads
}

func gatherCPUInfo(info *SystemInfo, wg *sync.WaitGroup, isFast bool) {
	defer wg.Done()

	if runtime.GOOS == "linux" {
		model, speed, ct := getLinuxCPUInfo()
		if model != "" {
			info.CPU = model
		}
		if speed != "" && info.CPUSpeed == "" {
			info.CPUSpeed = speed
		}
		if ct != "" {
			info.CoresThreads = ct
		}
	}

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

	if info.CoresThreads == "" || info.CoresThreads == "0/0" {
		cores, _ := cpu.Counts(false)
		threads, _ := cpu.Counts(true)
		info.CoresThreads = fmt.Sprintf("%d/%d", cores, threads)
	}

	if !isFast {
		percentages, err := cpu.Percent(0, false)
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
		// Fast registry lookup
		prodName := getWindowsRegValue(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "ProductName")
		displayVer := getWindowsRegValue(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "DisplayVersion")
		currentBuild := getWindowsRegValue(`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "CurrentBuild")
		if prodName != "" {
			prodName = strings.TrimPrefix(prodName, "Microsoft ")
			if displayVer != "" && currentBuild != "" {
				return fmt.Sprintf("%s %s (Build %s)", prodName, displayVer, currentBuild)
			} else if currentBuild != "" {
				return fmt.Sprintf("%s (Build %s)", prodName, currentBuild)
			}
			return prodName
		}
		productName := runCommand("wmic", "os", "get", "Caption")
		if productName != "" {
			lines := strings.Split(productName, "\n")
			if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
				return strings.TrimPrefix(strings.TrimSpace(lines[1]), "Microsoft ")
			}
		}
	case "darwin":
		// Fast plist reading
		if data, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist"); err == nil {
			prodVer := extractPlistString(data, "ProductUserVisibleVersion")
			if prodVer == "" {
				prodVer = extractPlistString(data, "ProductVersion")
			}
			buildVer := extractPlistString(data, "ProductBuildVersion")
			if prodVer != "" && buildVer != "" {
				return fmt.Sprintf("macOS %s (%s)", prodVer, buildVer)
			} else if prodVer != "" {
				return fmt.Sprintf("macOS %s", prodVer)
			}
		}
		productVersion := runCommand("sw_vers", "-productVersion")
		buildVersion := runCommand("sw_vers", "-buildVersion")
		if productVersion != "" {
			return fmt.Sprintf("macOS %s (%s)", productVersion, buildVersion)
		}
	case "freebsd", "openbsd", "netbsd":
		ostype := runCommand("sysctl", "-n", "kern.ostype")
		osrelease := runCommand("sysctl", "-n", "kern.osrelease")
		if ostype != "" && osrelease != "" {
			return fmt.Sprintf("%s %s", ostype, osrelease)
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
		if os.Getenv("WT_SESSION") != "" {
			return "Windows Terminal"
		} else if os.Getenv("PSModulePath") != "" {
			shellPath = "powershell"
		} else if os.Getenv("ComSpec") != "" {
			shellPath = "cmd"
		} else {
			return "Command Prompt"
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
		if psVer := getWindowsRegValue(`HKLM\SOFTWARE\Microsoft\PowerShell\3\PowerShellEngine`, "PowerShellVersion"); psVer != "" {
			version = psVer
		} else {
			version = runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.Major")
		}
	}

	titleName := titleCase(shellName)
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
		if name := getWindowsRegValue(`HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\0000`, "DriverDesc"); name != "" {
			return name
		}
		out := runCommand("wmic", "path", "win32_VideoController", "get", "Caption")
		if out != "" {
			lines := strings.Split(out, "\n")
			var gpus []string
			for _, line := range lines {
				t := strings.TrimSpace(line)
				if t != "" && t != "Caption" {
					gpus = append(gpus, t)
				}
			}
			if len(gpus) > 0 {
				return strings.Join(gpus, ", ")
			}
		}
		return "Unknown"
	case "linux":
		return getLinuxGPU()
	case "darwin":
		out := runCommand("system_profiler", "SPDisplaysDataType")
		if out != "" {
			lines := strings.Split(out, "\n")
			var gpus []string
			for _, line := range lines {
				if strings.Contains(line, "Chipset Model:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						gpu := strings.TrimSpace(parts[1])
						if gpu != "" {
							gpus = append(gpus, gpu)
						}
					}
				}
			}
			if len(gpus) > 0 {
				return strings.Join(gpus, ", ")
			}
		}
		cpuBrand := runCommand("sysctl", "-n", "machdep.cpu.brand_string")
		if strings.Contains(cpuBrand, "Apple") {
			return cpuBrand + " GPU"
		}
		return "Unknown"
	case "freebsd", "openbsd", "netbsd":
		return getLinuxGPU()
	}
	return "Unknown"
}

func getLinuxOpenPorts() string {
	portSet := make(map[int]struct{})
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[0] == "sl" {
				continue
			}
			// fields[3] is state: "0A" is TCP_LISTEN (10)
			if fields[3] == "0A" {
				addrParts := strings.Split(fields[1], ":")
				if len(addrParts) == 2 {
					if port, err := strconv.ParseInt(addrParts[1], 16, 64); err == nil && port > 0 {
						portSet[int(port)] = struct{}{}
					}
				}
			}
		}
		f.Close()
	}
	if len(portSet) == 0 {
		return "None"
	}
	ports := make([]int, 0, len(portSet))
	for p := range portSet {
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

func getOpenPorts() string {
	if runtime.GOOS == "linux" {
		if res := getLinuxOpenPorts(); res != "" {
			return res
		}
	}

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
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 100*time.Millisecond)
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
	if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return udpAddr.IP.String()
	}
	return "127.0.0.1"
}

func getLinuxResolution() string {
	// 1. Direct DRM sysfs connector inspection (Universal across Wayland, X11, GNOME, KDE, Sway, etc.)
	if files, err := os.ReadDir("/sys/class/drm"); err == nil {
		var resolutions []string
		for _, f := range files {
			name := f.Name()
			if strings.HasPrefix(name, "card") && strings.Contains(name, "-") {
				statusBytes, errSt := os.ReadFile("/sys/class/drm/" + name + "/status")
				if errSt != nil || strings.TrimSpace(string(statusBytes)) != "connected" {
					continue
				}
				modesBytes, errM := os.ReadFile("/sys/class/drm/" + name + "/modes")
				if errM == nil && len(modesBytes) > 0 {
					lines := strings.Split(strings.TrimSpace(string(modesBytes)), "\n")
					if len(lines) > 0 && lines[0] != "" {
						resolutions = append(resolutions, strings.TrimSpace(lines[0]))
					}
				}
			}
		}
		if len(resolutions) > 0 {
			return strings.Join(resolutions, ", ")
		}
	}

	// 2. Framebuffer sysfs fallback
	if fbModes, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size"); err == nil {
		s := strings.TrimSpace(string(fbModes))
		if s != "" {
			return strings.Replace(s, ",", "x", 1)
		}
	}

	// 3. Fallback: xrandr (if available on X11 / Xwayland)
	if out := runCommand("xrandr", "--current"); out != "" {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "*") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					return fields[0]
				}
			}
		}
	}

	return ""
}

func getResolution() string {
	var res string
	switch runtime.GOOS {
	case "windows":
		out := runCommand("wmic", "path", "Win32_VideoController", "get", "CurrentHorizontalResolution,CurrentVerticalResolution")
		if out != "" {
			lines := strings.Split(out, "\n")
			for _, l := range lines {
				f := strings.Fields(l)
				if len(f) == 2 && f[0] != "CurrentHorizontalResolution" {
					res = fmt.Sprintf("%sx%s", f[0], f[1])
					break
				}
			}
		}
		if res == "" {
			output := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_VideoController).CurrentHorizontalResolution,(Get-CimInstance Win32_VideoController).CurrentVerticalResolution -join 'x'")
			if output != "" {
				res = strings.TrimSpace(output)
			}
		}
	case "linux":
		res = getLinuxResolution()
	case "darwin":
		out := runCommand("system_profiler", "SPDisplaysDataType")
		if out != "" {
			lines := strings.Split(out, "\n")
			for _, l := range lines {
				if strings.Contains(l, "Resolution:") {
					parts := strings.SplitN(l, ":", 2)
					if len(parts) == 2 {
						fields := strings.Fields(parts[1])
						if len(fields) >= 3 {
							res = fmt.Sprintf("%s x %s", fields[0], fields[2])
							break
						}
						res = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}
	}
	if res == "" {
		return "Unknown"
	}
	return res
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
				return titleCase(cleanName)
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
		return titleCase(termProg)
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
		return titleCase(desktopSession)
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
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
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
				case "hyprland":
					return "Hyprland"
				}
				return "Wayland"
			}
		}
		return getLinuxWM()
	case "windows":
		return "DWM"
	case "darwin":
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
		if loc := getWindowsRegValue(`HKCU\Control Panel\International`, "LocaleName"); loc != "" {
			return loc
		}
		return runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-Culture).Name")
	}
	return "Unknown"
}

func getDesktopEnvironment() string {
	if runtime.GOOS == "windows" {
		return "Explorer"
	}
	if runtime.GOOS == "darwin" {
		return "Aqua"
	}
	de := os.Getenv("XDG_CURRENT_DESKTOP")
	if de == "" {
		de = os.Getenv("DESKTOP_SESSION")
	}
	de = strings.Replace(de, "plasmawayland", "Plasma (Wayland)", 1)
	de = strings.Replace(de, "plasma", "Plasma (X11)", 1)
	return titleCase(de)
}

func readSQLiteVarint(b []byte) (uint64, int) {
	var val uint64
	for i := 0; i < 9 && i < len(b); i++ {
		byteVal := b[i]
		if i == 8 {
			val = (val << 8) | uint64(byteVal)
			return val, 9
		}
		val = (val << 7) | uint64(byteVal&0x7f)
		if byteVal&0x80 == 0 {
			return val, i + 1
		}
	}
	return val, len(b)
}

func countSQLitePageRows(f *os.File, pageNum int, pageSize int) int {
	if pageNum <= 0 {
		return 0
	}
	offset := int64(pageNum-1) * int64(pageSize)
	buf := make([]byte, pageSize)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return 0
	}

	hdrOffset := 0
	if pageNum == 1 {
		hdrOffset = 100
	}
	if hdrOffset+8 > len(buf) {
		return 0
	}

	pageType := buf[hdrOffset]
	numCells := int(binary.BigEndian.Uint16(buf[hdrOffset+3 : hdrOffset+5]))

	switch pageType {
	case 0x0D: // Table leaf
		return numCells
	case 0x05: // Table interior
		total := 0
		if hdrOffset+12 > len(buf) {
			return 0
		}
		rightChild := int(binary.BigEndian.Uint32(buf[hdrOffset+8 : hdrOffset+12]))
		cellPtrsOffset := hdrOffset + 12
		for i := 0; i < numCells; i++ {
			pOffset := cellPtrsOffset + i*2
			if pOffset+2 > len(buf) {
				break
			}
			cellPtr := int(binary.BigEndian.Uint16(buf[pOffset : pOffset+2]))
			if cellPtr+4 <= len(buf) {
				leftChild := int(binary.BigEndian.Uint32(buf[cellPtr : cellPtr+4]))
				total += countSQLitePageRows(f, leftChild, pageSize)
			}
		}
		total += countSQLitePageRows(f, rightChild, pageSize)
		return total
	}
	return 0
}

func scanSQLiteSchemaPage(f *os.File, pageNum int, pageSize int, targetTable string) int {
	if pageNum <= 0 {
		return 0
	}
	offset := int64(pageNum-1) * int64(pageSize)
	buf := make([]byte, pageSize)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return 0
	}

	hdrOffset := 0
	if pageNum == 1 {
		hdrOffset = 100
	}
	if hdrOffset+8 > len(buf) {
		return 0
	}

	pageType := buf[hdrOffset]
	numCells := int(binary.BigEndian.Uint16(buf[hdrOffset+3 : hdrOffset+5]))

	if pageType == 0x05 { // Interior table
		rightChild := int(binary.BigEndian.Uint32(buf[hdrOffset+8 : hdrOffset+12]))
		cellPtrsOffset := hdrOffset + 12
		for i := 0; i < numCells; i++ {
			pOffset := cellPtrsOffset + i*2
			if pOffset+2 > len(buf) {
				break
			}
			cellPtr := int(binary.BigEndian.Uint16(buf[pOffset : pOffset+2]))
			if cellPtr+4 <= len(buf) {
				leftChild := int(binary.BigEndian.Uint32(buf[cellPtr : cellPtr+4]))
				if res := scanSQLiteSchemaPage(f, leftChild, pageSize, targetTable); res > 0 {
					return res
				}
			}
		}
		return scanSQLiteSchemaPage(f, rightChild, pageSize, targetTable)
	}

	if pageType == 0x0D { // Leaf table
		cellPtrsOffset := hdrOffset + 8
		for i := 0; i < numCells; i++ {
			pOffset := cellPtrsOffset + i*2
			if pOffset+2 > len(buf) {
				break
			}
			cellPtr := int(binary.BigEndian.Uint16(buf[pOffset : pOffset+2]))
			if cellPtr >= len(buf) {
				continue
			}
			cellData := buf[cellPtr:]
			if !bytes.Contains(cellData, []byte(targetTable)) {
				continue
			}

			payloadLen, n1 := readSQLiteVarint(cellData)
			if int(payloadLen) > len(cellData)-n1 {
				payloadLen = uint64(len(cellData) - n1)
			}
			_, n2 := readSQLiteVarint(cellData[n1:])
			recordOffset := n1 + n2
			if recordOffset >= len(cellData) {
				continue
			}

			record := cellData[recordOffset : recordOffset+int(payloadLen)-n2]
			hdrLen, n3 := readSQLiteVarint(record)
			if int(hdrLen) > len(record) {
				continue
			}
			recHdr := record[n3:hdrLen]

			var serialTypes []uint64
			for len(recHdr) > 0 {
				st, n := readSQLiteVarint(recHdr)
				serialTypes = append(serialTypes, st)
				recHdr = recHdr[n:]
			}

			if len(serialTypes) >= 4 {
				bodyOffset := int(hdrLen)
				currBody := record[bodyOffset:]
				var cols [][]byte
				for _, st := range serialTypes {
					var colLen int
					if st >= 12 && st%2 == 0 {
						colLen = int((st - 12) / 2)
					} else if st >= 13 && st%2 == 1 {
						colLen = int((st - 13) / 2)
					} else if st == 1 {
						colLen = 1
					} else if st == 2 {
						colLen = 2
					} else if st == 3 {
						colLen = 3
					} else if st == 4 {
						colLen = 4
					} else if st == 5 {
						colLen = 6
					} else if st == 6 || st == 7 {
						colLen = 8
					}
					if colLen > len(currBody) {
						break
					}
					cols = append(cols, currBody[:colLen])
					currBody = currBody[colLen:]
				}

				if len(cols) >= 4 {
					colType := string(cols[0])
					colName := string(cols[1])
					if (colType == "table" || colType == "index") && colName == targetTable {
						rootBytes := cols[3]
						var rootPage int
						for _, b := range rootBytes {
							rootPage = (rootPage << 8) | int(b)
						}
						return rootPage
					}
				}
			}
		}
	}
	return 0
}

func countRPMFromSQLite(dbPath string) int {
	f, err := os.Open(dbPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	header := make([]byte, 100)
	if _, err := io.ReadFull(f, header); err != nil {
		return 0
	}
	if string(header[:16]) != "SQLite format 3\x00" {
		return 0
	}

	pageSize := int(binary.BigEndian.Uint16(header[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}

	// Try Name table first (much faster as leaf cells are small)
	rootPage := scanSQLiteSchemaPage(f, 1, pageSize, "Name")
	if rootPage == 0 {
		rootPage = scanSQLiteSchemaPage(f, 1, pageSize, "Packages")
	}
	if rootPage == 0 {
		return 0
	}

	return countSQLitePageRows(f, rootPage, pageSize)
}

// GetPackageCounts fetches package counts across all supported package managers. (Exported)
func GetPackageCounts() string {
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
		{"Dpkg", func() int {
			data, err := os.ReadFile("/var/lib/dpkg/status")
			if err != nil {
				return 0
			}
			count := bytes.Count(data, []byte("\nPackage: "))
			if bytes.HasPrefix(data, []byte("Package: ")) {
				count++
			}
			return count
		}},
		{"RPM", func() int {
			// Fast path 1: Direct SQLite B-tree header counting (< 1ms)
			rpmDBPaths := []string{
				"/var/lib/rpm/rpmdb.sqlite",
				"/usr/lib/sysimage/rpm/rpmdb.sqlite",
				"/var/lib/rpm/Packages.db",
				"/usr/lib/sysimage/rpm/Packages.db",
			}
			for _, p := range rpmDBPaths {
				if count := countRPMFromSQLite(p); count > 0 {
					return count
				}
			}

			// Fast path 2: Non-verifying rpm query with tight timeout
			if _, err := exec.LookPath("rpm"); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
				defer cancel()
				cmd := exec.CommandContext(ctx, "rpm", "-qa", "--nodigest", "--nosignature", "--qf", ".\n")
				if out, err := cmd.Output(); err == nil {
					count := bytes.Count(out, []byte("\n"))
					if count > 0 {
						return count
					}
				}
			}
			return 0
		}},
		{"APK", func() int {
			data, err := os.ReadFile("/lib/apk/db/installed")
			if err != nil {
				return 0
			}
			count := bytes.Count(data, []byte("\nP:"))
			if bytes.HasPrefix(data, []byte("P:")) {
				count++
			}
			return count
		}},
		{"XBPS", func() int {
			dirs, err := os.ReadDir("/var/db/xbps")
			if err != nil {
				return 0
			}
			count := 0
			for _, d := range dirs {
				if strings.HasPrefix(d.Name(), "pkg-") || strings.HasSuffix(d.Name(), ".plist") {
					count++
				}
			}
			return count
		}},
		{"Portage", func() int {
			cats, err := os.ReadDir("/var/db/pkg")
			if err != nil {
				return 0
			}
			count := 0
			for _, cat := range cats {
				if cat.IsDir() && !strings.HasPrefix(cat.Name(), ".") {
					if pkgs, err := os.ReadDir("/var/db/pkg/" + cat.Name()); err == nil {
						for _, p := range pkgs {
							if p.IsDir() && !strings.HasPrefix(p.Name(), ".") {
								count++
							}
						}
					}
				}
			}
			return count
		}},
		{"Nix", func() int {
			count := 0
			for _, p := range []string{"/nix/var/nix/profiles/default", "/nix/var/nix/profiles/system"} {
				if dirs, err := os.ReadDir(p); err == nil {
					for _, d := range dirs {
						if !strings.HasPrefix(d.Name(), ".") {
							count++
						}
					}
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
		{"MacPorts", func() int {
			count := 0
			if dirs, err := os.ReadDir("/opt/local/var/macports/software"); err == nil {
				for _, d := range dirs {
					if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
						count++
					}
				}
			}
			return count
		}},
		{"Scoop", func() int {
			count := 0
			home := os.Getenv("USERPROFILE")
			if home == "" {
				home = os.Getenv("HOME")
			}
			if home != "" {
				if dirs, err := os.ReadDir(home + "/scoop/apps"); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") && d.Name() != "scoop" {
							count++
						}
					}
				}
			}
			return count
		}},
		{"Chocolatey", func() int {
			count := 0
			chocoPaths := []string{
				os.Getenv("ChocolateyInstall") + "/lib",
				"C:/ProgramData/chocolatey/lib",
			}
			for _, p := range chocoPaths {
				if p == "/lib" || p == "" {
					continue
				}
				if dirs, err := os.ReadDir(p); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") && !strings.EqualFold(d.Name(), "chocolatey") {
							count++
						}
					}
					if count > 0 {
						break
					}
				}
			}
			return count
		}},
		{"Winget", func() int {
			count := 0
			localAppData := os.Getenv("LOCALAPPDATA")
			if localAppData != "" {
				if dirs, err := os.ReadDir(localAppData + "/Microsoft/WinGet/Packages"); err == nil {
					for _, d := range dirs {
						if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
							count++
						}
					}
				}
			}
			return count
		}},
		{"BSD Pkg", func() int {
			if runtime.GOOS != "freebsd" && runtime.GOOS != "openbsd" && runtime.GOOS != "netbsd" {
				return 0
			}
			if count := countRPMFromSQLite("/var/db/pkg/local.sqlite"); count > 0 {
				return count
			}
			dirs, err := os.ReadDir("/var/db/pkg")
			if err != nil {
				return 0
			}
			count := 0
			for _, d := range dirs {
				if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
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

func cleanISPName(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip AS number prefixes like "AS55836 "
	if strings.HasPrefix(raw, "AS") {
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return raw
}

func getPublicIPInfo() (ip, isp, city, country string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	client := http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	// Provider 1: ipinfo.io (HTTPS, detailed and reliable)
	req1, errReq1 := http.NewRequestWithContext(ctx, "GET", "https://ipinfo.io/json", nil)
	if errReq1 == nil {
		req1.Header.Set("User-Agent", "KernelView-Go")
		if resp1, err1 := client.Do(req1); err1 == nil {
			var ipInfo struct {
				IP      string `json:"ip"`
				City    string `json:"city"`
				Region  string `json:"region"`
				Country string `json:"country"`
				Org     string `json:"org"`
			}
			errDecode := json.NewDecoder(io.LimitReader(resp1.Body, 64*1024)).Decode(&ipInfo)
			resp1.Body.Close()
			if errDecode == nil && ipInfo.IP != "" {
				c := ipInfo.City
				if ipInfo.Region != "" && ipInfo.Region != ipInfo.City {
					c = fmt.Sprintf("%s, %s", ipInfo.City, ipInfo.Region)
				}
				return ipInfo.IP, cleanISPName(ipInfo.Org), c, ipInfo.Country, nil
			}
		}
	}

	// Provider 2: ip-api.com (fallback)
	req2, errReq2 := http.NewRequestWithContext(ctx, "GET", "https://ip-api.com/json/", nil)
	if errReq2 == nil {
		req2.Header.Set("User-Agent", "KernelView-Go")
		if resp2, err2 := client.Do(req2); err2 == nil {
			var apiResp ipAPIResponse
			errDecode := json.NewDecoder(io.LimitReader(resp2.Body, 64*1024)).Decode(&apiResp)
			resp2.Body.Close()
			if errDecode == nil && apiResp.Status == "success" {
				return apiResp.Query, apiResp.ISP, apiResp.City, apiResp.Country, nil
			}
		}
	}

	// Provider 3: api.ipify.org (minimal fallback)
	req3, errReq3 := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org?format=json", nil)
	if errReq3 == nil {
		req3.Header.Set("User-Agent", "KernelView-Go")
		if resp3, err3 := client.Do(req3); err3 == nil {
			var ipifyResp struct {
				IP string `json:"ip"`
			}
			errDecode := json.NewDecoder(io.LimitReader(resp3.Body, 16*1024)).Decode(&ipifyResp)
			resp3.Body.Close()
			if errDecode == nil && ipifyResp.IP != "" {
				return ipifyResp.IP, "Unknown ISP", "Unknown City", "Unknown Country", nil
			}
		}
	}

	return "", "", "", "", fmt.Errorf("all public IP lookup endpoints failed")
}

func getDefaultGateway() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/net/route")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) >= 3 && fields[1] == "00000000" {
					gwHex := fields[2]
					if len(gwHex) == 8 && gwHex != "00000000" {
						d, err := strconv.ParseUint(gwHex, 16, 32)
						if err == nil {
							ip := net.IPv4(byte(d), byte(d>>8), byte(d>>16), byte(d>>24))
							return fmt.Sprintf("%s (%s)", ip.String(), fields[0])
						}
					}
				}
			}
		}
	} else if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
		out := runCommand("route", "-n", "get", "default")
		var gw, iface string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "gateway:") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					gw = strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(line, "interface:") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					iface = strings.TrimSpace(parts[1])
				}
			}
		}
		if gw != "" {
			if iface != "" {
				return fmt.Sprintf("%s (%s)", gw, iface)
			}
			return gw
		}
	} else if runtime.GOOS == "windows" {
		out := runCommand("route", "print", "0.0.0.0")
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 5 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
				return fmt.Sprintf("%s (%s)", fields[2], fields[3])
			}
		}
		psOut := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Select-Object -First 1 -ExpandProperty NextHop")
		if psOut != "" {
			return psOut
		}
	}
	return "N/A"
}

func sliceContains(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

func getWifiDetails(ifaceName string) *WifiInfo {
	if runtime.GOOS == "linux" {
		out := runCommand("iw", "dev", ifaceName, "link")
		if out != "" && strings.Contains(out, "SSID:") {
			wifi := &WifiInfo{}
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Connected to") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						wifi.BSSID = fields[2]
					}
				} else if strings.HasPrefix(line, "SSID:") {
					wifi.SSID = strings.TrimSpace(strings.TrimPrefix(line, "SSID:"))
				} else if strings.HasPrefix(line, "signal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if dbm, err := strconv.Atoi(fields[1]); err == nil {
							wifi.SignalDBm = dbm
							perc := 2 * (dbm + 100)
							if perc > 100 {
								perc = 100
							} else if perc < 0 {
								perc = 0
							}
							wifi.SignalPerc = perc
						}
					}
				} else if strings.HasPrefix(line, "freq:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						freqMHz, _ := strconv.ParseFloat(fields[1], 64)
						channel := int((freqMHz - 2407) / 5)
						if freqMHz > 5000 {
							channel = int((freqMHz - 5000) / 5)
						}
						wifi.Freq = fmt.Sprintf("%.3f GHz", freqMHz/1000.0)
						wifi.Channel = fmt.Sprintf("%d", channel)
					}
				} else if strings.HasPrefix(line, "rx bitrate:") || strings.HasPrefix(line, "tx bitrate:") {
					fields := strings.Fields(line)
					if len(fields) >= 4 {
						speed := fmt.Sprintf("%s %s", fields[2], fields[3])
						dir := "RX"
						if strings.HasPrefix(line, "tx") {
							dir = "TX"
						}
						if wifi.Bitrate == "" {
							wifi.Bitrate = fmt.Sprintf("%s (%s)", speed, dir)
						} else {
							wifi.Bitrate += fmt.Sprintf(" │ %s (%s)", speed, dir)
						}
					}
				}
			}
			return wifi
		}
		// Fallback: nmcli
		out = runCommand("nmcli", "-t", "-f", "active,ssid,bssid,signal,freq,chan", "dev", "wifi")
		if out != "" {
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "yes:") {
					parts := strings.Split(line, ":")
					if len(parts) >= 6 {
						sig, _ := strconv.Atoi(parts[3])
						return &WifiInfo{
							SSID:       parts[1],
							BSSID:      parts[2],
							SignalPerc: sig,
							Freq:       fmt.Sprintf("%s (Ch %s)", parts[4], parts[5]),
						}
					}
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		out := runCommand("/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport", "-I")
		if out != "" && strings.Contains(out, "SSID:") {
			wifi := &WifiInfo{}
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "SSID: ") {
					wifi.SSID = strings.TrimPrefix(line, "SSID: ")
				} else if strings.HasPrefix(line, "BSSID: ") {
					wifi.BSSID = strings.TrimPrefix(line, "BSSID: ")
				} else if strings.HasPrefix(line, "agrCtlRSSI: ") {
					rssi, _ := strconv.Atoi(strings.TrimPrefix(line, "agrCtlRSSI: "))
					wifi.SignalDBm = rssi
					perc := 2 * (rssi + 100)
					if perc > 100 {
						perc = 100
					} else if perc < 0 {
						perc = 0
					}
					wifi.SignalPerc = perc
				} else if strings.HasPrefix(line, "channel: ") {
					ch := strings.TrimPrefix(line, "channel: ")
					if parts := strings.Split(ch, ","); len(parts) > 0 {
						wifi.Channel = parts[0]
					}
				} else if strings.HasPrefix(line, "lastTxRate: ") {
					wifi.Bitrate = strings.TrimPrefix(line, "lastTxRate: ") + " Mbps"
				} else if strings.HasPrefix(line, "link auth: ") {
					wifi.Security = strings.TrimPrefix(line, "link auth: ")
				}
			}
			return wifi
		}
		// Fallback for macOS Sonoma / Sequoia
		ssidOut := runCommand("networksetup", "-getairportnetwork", ifaceName)
		if ssidOut != "" && strings.Contains(ssidOut, "Current Wi-Fi Network:") {
			wifi := &WifiInfo{}
			wifi.SSID = strings.TrimSpace(strings.TrimPrefix(ssidOut, "Current Wi-Fi Network:"))
			return wifi
		}
	} else if runtime.GOOS == "windows" {
		out := runCommand("netsh", "wlan", "show", "interfaces")
		if out != "" && strings.Contains(out, "SSID") {
			wifi := &WifiInfo{}
			var rxRate, txRate string
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])

				switch key {
				case "SSID":
					wifi.SSID = val
				case "BSSID":
					wifi.BSSID = val
				case "Signal":
					sigStr := strings.TrimSuffix(val, "%")
					if sig, err := strconv.Atoi(sigStr); err == nil {
						wifi.SignalPerc = sig
						wifi.SignalDBm = int(float64(sig)/2.0 - 100.0)
					}
				case "Radio type":
					wifi.Freq = val
				case "Channel":
					wifi.Channel = val
				case "Receive rate (Mbps)":
					rxRate = val + " Mbps (RX)"
				case "Transmit rate (Mbps)":
					txRate = val + " Mbps (TX)"
				case "Authentication":
					wifi.Security = val
				}
			}
			if rxRate != "" && txRate != "" {
				wifi.Bitrate = fmt.Sprintf("%s │ %s", rxRate, txRate)
			} else if rxRate != "" {
				wifi.Bitrate = rxRate
			} else if txRate != "" {
				wifi.Bitrate = txRate
			}
			return wifi
		}
	}
	return nil
}

func getSocketStats() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/net/sockstat")
		if err == nil {
			var tcpActive, udpActive, totalSockets int
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "sockets:") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						totalSockets, _ = strconv.Atoi(fields[2])
					}
				} else if strings.HasPrefix(line, "TCP:") {
					fields := strings.Fields(line)
					for i, f := range fields {
						if f == "inuse" && i+1 < len(fields) {
							tcpActive, _ = strconv.Atoi(fields[i+1])
						}
					}
				} else if strings.HasPrefix(line, "UDP:") {
					fields := strings.Fields(line)
					for i, f := range fields {
						if f == "inuse" && i+1 < len(fields) {
							udpActive, _ = strconv.Atoi(fields[i+1])
						}
					}
				}
			}
			if totalSockets > 0 || tcpActive > 0 {
				return fmt.Sprintf("TCP: %d active │ UDP: %d active │ Sockets: %d", tcpActive, udpActive, totalSockets)
			}
		}
	}

	// Cross-platform fallback for Windows, macOS, FreeBSD, OpenBSD: netstat -an
	out := runCommand("netstat", "-an")
	if out != "" {
		var tcpCount, udpCount int
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "tcp") {
				tcpCount++
			} else if strings.HasPrefix(lower, "udp") {
				udpCount++
			}
		}
		if tcpCount > 0 || udpCount > 0 {
			return fmt.Sprintf("TCP: %d active │ UDP: %d active │ Total: %d", tcpCount, udpCount, tcpCount+udpCount)
		}
	}

	return ""
}

func formatNumber(n uint64) string {
	in := strconv.FormatUint(n, 10)
	out := make([]byte, len(in)+(len(in)-1)/3)
	for i, j, k := len(in)-1, len(out)-1, 0; i >= 0; i, j = i-1, j-1 {
		out[j] = in[i]
		k++
		if k%3 == 0 && j > 0 {
			j--
			out[j] = ','
		}
	}
	return string(out)
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
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
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
		if runtime.GOOS == "darwin" && len(dnsServers) == 0 {
			out := runCommand("scutil", "--dns")
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "nameserver[") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						ns := strings.TrimSpace(parts[1])
						if ns != "" && !sliceContains(dnsServers, ns) {
							dnsServers = append(dnsServers, ns)
						}
					}
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		out := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object {$_.ServerAddresses} | Select-Object -ExpandProperty ServerAddresses")
		if out != "" {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && net.ParseIP(line) != nil && !sliceContains(dnsServers, line) {
					dnsServers = append(dnsServers, line)
				}
			}
		}
		if len(dnsServers) == 0 {
			out := runCommand("ipconfig", "/all")
			capture := false
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if strings.Contains(line, "DNS Servers") {
					parts := strings.Split(line, ":")
					if len(parts) > 1 {
						ns := strings.TrimSpace(parts[1])
						if net.ParseIP(ns) != nil && !sliceContains(dnsServers, ns) {
							dnsServers = append(dnsServers, ns)
						}
					}
					capture = true
				} else if capture {
					if strings.Contains(line, ":") {
						capture = false
					} else {
						ns := strings.TrimSpace(line)
						if net.ParseIP(ns) != nil && !sliceContains(dnsServers, ns) {
							dnsServers = append(dnsServers, ns)
						}
					}
				}
			}
		}
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
	if !reValidHost.MatchString(host) {
		return "Failed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", "400", host)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-t", "1", host)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", host)
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
		"Resolution": getResolution,
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
			"Packages":    GetPackageCounts,
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

	numWorkers := 16
	if len(allProcs) < numWorkers {
		numWorkers = len(allProcs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Step 1: Prime the CPU percent calculation by calling it once on all processes.
	jobs1 := make(chan *process.Process, len(allProcs))
	for _, p := range allProcs {
		if p != nil {
			jobs1 <- p
		}
	}
	close(jobs1)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }()
			for proc := range jobs1 {
				_, _ = proc.CPUPercent()
			}
		}()
	}
	wg.Wait()

	// Step 2: Sleep to allow CPU usage to accumulate.
	time.Sleep(100 * time.Millisecond)

	// Step 3: Call CPUPercent on the same instances to get the actual CPU usage.
	jobs2 := make(chan *process.Process, len(allProcs))
	for _, p := range allProcs {
		if p != nil {
			jobs2 <- p
		}
	}
	close(jobs2)

	results := make(chan ProcessInfo, len(allProcs))
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }()
			for proc := range jobs2 {
				pid := proc.Pid
				name, err := proc.Name()
				if err != nil || name == "" || pid <= 1 {
					continue
				}

				// Exclude common system/idle processes to keep the list clean
				if runtime.GOOS == "linux" && (strings.HasPrefix(name, "[") || name == "systemd" || name == "init") {
					continue
				}
				if name == "kernel_task" || name == "idle" {
					continue
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
			}
		}()
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

	wg.Add(4)

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

	go func() {
		defer wg.Done()
		info.Gateway = getDefaultGateway()
		info.SocketStats = getSocketStats()
	}()

	// Inspect local interfaces
	info.Hostname, _ = os.Hostname()
	info.Proxy = getProxyInfo()

	ifaces, err := net.Interfaces()
	if err == nil {
		var ifaceDetails []NetworkInterfaceDetail
		var primaryName string
		var totalRx, totalTx, totalRxPackets, totalTxPackets, totalRxErr, totalTxErr uint64

		var ioMap map[string]psnet.IOCountersStat
		if counters, errC := psnet.IOCounters(true); errC == nil {
			ioMap = make(map[string]psnet.IOCountersStat, len(counters))
			for _, c := range counters {
				ioMap[c.Name] = c
			}
		}

		gwIface := ""
		if strings.Contains(info.Gateway, "(") {
			parts := strings.Split(info.Gateway, "(")
			if len(parts) > 1 {
				gwIface = strings.TrimSuffix(parts[1], ")")
			}
		}

		for _, iface := range ifaces {
			detail := NetworkInterfaceDetail{
				Name: iface.Name,
				MAC:  iface.HardwareAddr.String(),
				MTU:  iface.MTU,
			}

			if iface.Flags&net.FlagUp != 0 {
				detail.State = "UP"
			} else {
				detail.State = "DOWN"
			}

			nameLower := strings.ToLower(iface.Name)
			if iface.Flags&net.FlagLoopback != 0 || nameLower == "lo" {
				detail.Type = "Loopback"
			} else if strings.HasPrefix(nameLower, "wl") || strings.Contains(nameLower, "wifi") || strings.HasPrefix(nameLower, "wlan") || strings.HasPrefix(nameLower, "ath") {
				detail.Type = "Wi-Fi"
			} else if strings.HasPrefix(nameLower, "en") || strings.HasPrefix(nameLower, "eth") {
				detail.Type = "Ethernet"
			} else if strings.Contains(nameLower, "vir") || strings.Contains(nameLower, "docker") || strings.HasPrefix(nameLower, "veth") || strings.HasPrefix(nameLower, "br") || strings.HasPrefix(nameLower, "tun") || strings.HasPrefix(nameLower, "tap") {
				detail.Type = "Virtual"
			} else {
				detail.Type = "Network"
			}

			addrs, errAddr := iface.Addrs()
			if errAddr == nil {
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
						if ip.To4() != nil && detail.IPv4 == "" {
							detail.IPv4 = v.String()
						} else if ip.To4() == nil && detail.IPv6 == "" && !ip.IsLinkLocalUnicast() {
							detail.IPv6 = v.String()
						}
					case *net.IPAddr:
						ip = v.IP
						if ip.To4() != nil && detail.IPv4 == "" {
							detail.IPv4 = ip.String()
						} else if ip.To4() == nil && detail.IPv6 == "" && !ip.IsLinkLocalUnicast() {
							detail.IPv6 = ip.String()
						}
					}
				}
			}

			if runtime.GOOS == "linux" {
				statsDir := fmt.Sprintf("/sys/class/net/%s/statistics", iface.Name)
				readStat := func(file string) uint64 {
					b, err := os.ReadFile(fmt.Sprintf("%s/%s", statsDir, file))
					if err == nil {
						v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
						return v
					}
					return 0
				}
				detail.RxBytes = readStat("rx_bytes")
				detail.TxBytes = readStat("tx_bytes")
				detail.RxPackets = readStat("rx_packets")
				detail.TxPackets = readStat("tx_packets")
				detail.RxErrors = readStat("rx_errors")
				detail.TxErrors = readStat("tx_errors")
				detail.RxDropped = readStat("rx_dropped")
				detail.TxDropped = readStat("tx_dropped")

				speedBytes, errS := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/speed", iface.Name))
				if errS == nil {
					s := strings.TrimSpace(string(speedBytes))
					if s != "" && s != "-1" {
						detail.Speed = s + " Mbps"
					}
				}
			}

			// Cross-platform fallback for Windows, macOS, BSD, or if Linux sysfs didn't have stats
			if detail.RxBytes == 0 && detail.TxBytes == 0 && ioMap != nil {
				if c, ok := ioMap[iface.Name]; ok {
					detail.RxBytes = c.BytesRecv
					detail.TxBytes = c.BytesSent
					detail.RxPackets = c.PacketsRecv
					detail.TxPackets = c.PacketsSent
					detail.RxErrors = c.Errin
					detail.TxErrors = c.Errout
					detail.RxDropped = c.Dropin
					detail.TxDropped = c.Dropout
				}
			}

			if detail.Type != "Loopback" {
				totalRx += detail.RxBytes
				totalTx += detail.TxBytes
				totalRxPackets += detail.RxPackets
				totalTxPackets += detail.TxPackets
				totalRxErr += detail.RxErrors
				totalTxErr += detail.TxErrors
			}

			ifaceDetails = append(ifaceDetails, detail)

			isPrimary := false
			if gwIface != "" && iface.Name == gwIface {
				isPrimary = true
			} else if primaryName == "" && detail.State == "UP" && detail.Type != "Loopback" && detail.Type != "Virtual" && detail.IPv4 != "" {
				isPrimary = true
			}

			if isPrimary {
				primaryName = iface.Name
				info.PrimaryIface = iface.Name
				info.IfaceType = detail.Type
				info.PrivateIP = detail.IPv4
				info.IPv6Address = detail.IPv6
				info.MACAddress = detail.MAC
				if detail.Type == "Wi-Fi" {
					info.Wifi = getWifiDetails(iface.Name)
				}
			}
		}

		info.Interfaces = ifaceDetails
		info.RxTotal = FormatBytes(totalRx)
		info.TxTotal = FormatBytes(totalTx)
		if totalRxPackets > 0 || totalTxPackets > 0 {
			info.PacketStats = fmt.Sprintf("%s RX │ %s TX (%d errors)", formatNumber(totalRxPackets), formatNumber(totalTxPackets), totalRxErr+totalTxErr)
		}
	}

	wg.Wait()

	if errPublicIP != nil || info.PublicIP == "" {
		info.PublicIP = "Unavailable"
	}
	if info.ISP == "" {
		info.ISP = "Unknown"
	}
	if info.PrivateIP == "" {
		info.PrivateIP = getIPAddress()
	}
	if info.MACAddress == "" {
		info.MACAddress = getPrimaryMACAddress()
	}
	info.IOCounters = getIOCounters()

	return info, nil
}

// DiskPartitionInfo holds filesystem mount details (Exported)
type DiskPartitionInfo struct {
	Mountpoint  string
	Device      string
	Fstype      string
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

// DiskDeviceIO holds per-device IO metrics (Exported)
type DiskDeviceIO struct {
	Name       string
	ReadSpeed  float64 // bytes/sec
	WriteSpeed float64 // bytes/sec
	ReadIOPS   float64 // ops/sec
	WriteIOPS  float64 // ops/sec
	ReadTotal  uint64  // bytes
	WriteTotal uint64  // bytes
}

// LiveMetrics holds real-time system stats (Exported)
type LiveMetrics struct {
	Uptime         string
	CPUUsage       float64
	CPUCores       []float64 // Per-core CPU percentages (Exported)
	RAMUsed        uint64
	RAMTotal       uint64
	RAMPercent     float64
	SwapUsed       uint64
	SwapTotal      uint64
	SwapPercent    float64
	DiskUsed       uint64
	DiskTotal      uint64
	DiskPercent    float64
	DiskReadSpeed  float64 // bytes/sec overall
	DiskWriteSpeed float64 // bytes/sec overall
	DiskReadTotal  uint64  // bytes overall
	DiskWriteTotal uint64  // bytes overall
	DiskPartitions []DiskPartitionInfo
	DiskDevices    []DiskDeviceIO
	Temperature    float64
	NetRxSpeed     float64 // bytes/sec
	NetTxSpeed     float64 // bytes/sec
	NetRxTotal     uint64  // bytes
	NetTxTotal     uint64  // bytes
	NetIface       string
	Processes      []ProcessInfo
	GPUMetrics     LiveGPUMetrics // GPU telemetry details (Exported)
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

	// Cached disk partitions
	cachedPartitions     []DiskPartitionInfo
	lastPartitionsUpdate time.Time

	// Disk I/O tracking
	prevDiskIOCounters map[string]disk.IOCountersStat
	prevDiskTime       time.Time
	smoothedDiskRead   float64
	smoothedDiskWrite  float64

	// Cached temperature
	cachedTemp     float64
	lastTempUpdate time.Time

	// Cached net interface name
	cachedNetIface  string
	lastIfaceUpdate time.Time

	// Cached GPU metrics
	cachedGPUMetrics LiveGPUMetrics
	lastGPUUpdate    time.Time
	cachedDRMPaths   []string
	lastDRMPathsScan time.Time
	prevDRMEngineNS  uint64
	prevDRMTime      time.Time
	smoothedGPUUsage float64

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
		procsMap:           make(map[int32]*process.Process),
		prevDiskIOCounters: make(map[string]disk.IOCountersStat),
		bootTime:           bTime,
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

	// 7. GPU Telemetry (Real-time Linux DRM engine tracking with cross-vendor fallback)
	var gpuMetrics LiveGPUMetrics
	hasRealtimeDRM := false

	if drmM, ok := lt.getLinuxDRMGPU(); ok {
		gpuMetrics = drmM
		hasRealtimeDRM = true
	}

	if !hasRealtimeDRM {
		if time.Since(lt.lastGPUUpdate) >= 1*time.Second || lt.lastGPUUpdate.IsZero() {
			gm, err := getGPUMetrics()
			if err == nil {
				lt.cachedGPUMetrics = gm
			}
			lt.lastGPUUpdate = time.Now()
		}
		gpuMetrics = lt.cachedGPUMetrics
	} else {
		// Periodically refresh temperature from hwmon
		if time.Since(lt.lastGPUUpdate) >= 2*time.Second || lt.lastGPUUpdate.IsZero() {
			gpuMetrics.GPUTemp = getAMDOrIntelTemp("")
			lt.cachedGPUMetrics.GPUTemp = gpuMetrics.GPUTemp
			lt.lastGPUUpdate = time.Now()
		} else {
			gpuMetrics.GPUTemp = lt.cachedGPUMetrics.GPUTemp
		}
		if gpuMetrics.GPUMemTotal == 0 && metrics.RAMTotal > 0 {
			gpuMetrics.GPUMemTotal = metrics.RAMTotal / (1024 * 1024)
			if gpuMetrics.GPUMemTotal > 0 {
				gpuMetrics.GPUMemUsage = (float64(gpuMetrics.GPUMemUsed) / float64(gpuMetrics.GPUMemTotal)) * 100.0
			}
		}
	}
	metrics.GPUMetrics = gpuMetrics

	// 8. Processes (asynchronous background scanning for zero UI delay)
	if (time.Since(lt.lastProcUpdate) >= 1500*time.Millisecond || len(lt.cachedProcesses) == 0) && !lt.procScanning {
		lt.procScanning = true
		go lt.scanProcessesAsync()
	}
	metrics.Processes = lt.cachedProcesses

	// 9. Disk I/O & Partitions
	if time.Since(lt.lastPartitionsUpdate) >= 5*time.Second || lt.lastPartitionsUpdate.IsZero() {
		parts, err := disk.Partitions(false)
		if err == nil {
			var pList []DiskPartitionInfo
			seenMounts := make(map[string]bool)
			for _, p := range parts {
				if seenMounts[p.Mountpoint] {
					continue
				}
				if strings.HasPrefix(p.Mountpoint, "/proc") ||
					strings.HasPrefix(p.Mountpoint, "/sys") ||
					strings.HasPrefix(p.Mountpoint, "/dev") ||
					p.Fstype == "squashfs" {
					continue
				}
				u, err := disk.Usage(p.Mountpoint)
				if err == nil && u.Total > 0 {
					pList = append(pList, DiskPartitionInfo{
						Mountpoint:  p.Mountpoint,
						Device:      p.Device,
						Fstype:      p.Fstype,
						Total:       u.Total,
						Used:        u.Used,
						Free:        u.Free,
						UsedPercent: u.UsedPercent,
					})
					seenMounts[p.Mountpoint] = true
				}
			}
			sort.Slice(pList, func(i, j int) bool {
				if pList[i].Mountpoint == "/" {
					return true
				}
				if pList[j].Mountpoint == "/" {
					return false
				}
				return pList[i].Mountpoint < pList[j].Mountpoint
			})
			lt.cachedPartitions = pList
			lt.lastPartitionsUpdate = time.Now()
		}
	}
	metrics.DiskPartitions = lt.cachedPartitions

	ioCounters, err := disk.IOCounters()
	if err == nil && len(ioCounters) > 0 {
		var devList []DiskDeviceIO
		nowDisk := time.Now()
		elapsedDisk := 0.0
		if !lt.prevDiskTime.IsZero() {
			elapsedDisk = nowDisk.Sub(lt.prevDiskTime).Seconds()
		}

		var totalReadBytes, totalWriteBytes uint64
		var totalReadSpeed, totalWriteSpeed float64

		isBaseDisk := func(name string) bool {
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
				return false
			}
			if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") || strings.HasPrefix(name, "hd") {
				lastChar := name[len(name)-1]
				return lastChar < '0' || lastChar > '9'
			}
			if strings.HasPrefix(name, "nvme") {
				return !strings.Contains(name, "p")
			}
			if strings.HasPrefix(name, "mmcblk") {
				return !strings.Contains(name, "p")
			}
			return true
		}

		baseDiskCount := 0
		for name := range ioCounters {
			if isBaseDisk(name) {
				baseDiskCount++
			}
		}

		var devNames []string
		for name := range ioCounters {
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
				continue
			}
			devNames = append(devNames, name)
		}
		sort.Strings(devNames)

		for _, name := range devNames {
			curr := ioCounters[name]
			var rSpeed, wSpeed, rIOPS, wIOPS float64
			if prev, exists := lt.prevDiskIOCounters[name]; exists && elapsedDisk > 0 {
				if curr.ReadBytes >= prev.ReadBytes {
					rSpeed = float64(curr.ReadBytes-prev.ReadBytes) / elapsedDisk
				}
				if curr.WriteBytes >= prev.WriteBytes {
					wSpeed = float64(curr.WriteBytes-prev.WriteBytes) / elapsedDisk
				}
				if curr.ReadCount >= prev.ReadCount {
					rIOPS = float64(curr.ReadCount-prev.ReadCount) / elapsedDisk
				}
				if curr.WriteCount >= prev.WriteCount {
					wIOPS = float64(curr.WriteCount-prev.WriteCount) / elapsedDisk
				}
			}

			devList = append(devList, DiskDeviceIO{
				Name:       name,
				ReadSpeed:  rSpeed,
				WriteSpeed: wSpeed,
				ReadIOPS:   rIOPS,
				WriteIOPS:  wIOPS,
				ReadTotal:  curr.ReadBytes,
				WriteTotal: curr.WriteBytes,
			})

			if baseDiskCount == 0 || isBaseDisk(name) {
				totalReadBytes += curr.ReadBytes
				totalWriteBytes += curr.WriteBytes
				totalReadSpeed += rSpeed
				totalWriteSpeed += wSpeed
			}
		}

		sort.Slice(devList, func(i, j int) bool {
			baseI := isBaseDisk(devList[i].Name)
			baseJ := isBaseDisk(devList[j].Name)
			if baseI != baseJ {
				return baseI
			}
			actI := devList[i].ReadSpeed + devList[i].WriteSpeed
			actJ := devList[j].ReadSpeed + devList[j].WriteSpeed
			if actI != actJ {
				return actI > actJ
			}
			return devList[i].Name < devList[j].Name
		})

		if lt.smoothedDiskRead == 0 {
			lt.smoothedDiskRead = totalReadSpeed
		} else {
			lt.smoothedDiskRead = 0.50*totalReadSpeed + 0.50*lt.smoothedDiskRead
		}

		if lt.smoothedDiskWrite == 0 {
			lt.smoothedDiskWrite = totalWriteSpeed
		} else {
			lt.smoothedDiskWrite = 0.50*totalWriteSpeed + 0.50*lt.smoothedDiskWrite
		}

		metrics.DiskReadSpeed = lt.smoothedDiskRead
		metrics.DiskWriteSpeed = lt.smoothedDiskWrite
		metrics.DiskReadTotal = totalReadBytes
		metrics.DiskWriteTotal = totalWriteBytes
		metrics.DiskDevices = devList

		lt.prevDiskIOCounters = ioCounters
		lt.prevDiskTime = nowDisk
	}

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

	numWorkers := 16
	if len(procs) < numWorkers {
		numWorkers = len(procs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan *process.Process, len(procs))
	for _, p := range procs {
		jobs <- p
	}
	close(jobs)

	var wg sync.WaitGroup
	results := make(chan ProcessInfo, len(procs))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }()
			for p := range jobs {
				pid := p.Pid
				name, err := p.Name()
				if err != nil || name == "" || pid <= 1 {
					continue
				}
				if runtime.GOOS == "linux" && (strings.HasPrefix(name, "[") || name == "systemd" || name == "init") {
					continue
				}
				if name == "kernel_task" || name == "idle" {
					continue
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
			}
		}()
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

func (lt *LiveTracker) getLinuxDRMGPU() (LiveGPUMetrics, bool) {
	if runtime.GOOS != "linux" {
		return LiveGPUMetrics{}, false
	}

	// 1. Refresh cached DRM fdinfo paths if empty or after 15 seconds
	if time.Since(lt.lastDRMPathsScan) >= 15*time.Second || len(lt.cachedDRMPaths) == 0 {
		lt.cachedDRMPaths = lt.cachedDRMPaths[:0]
		entries, err := os.ReadDir("/proc")
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				if len(name) == 0 || name[0] < '0' || name[0] > '9' {
					continue
				}
				fdDir := "/proc/" + name + "/fdinfo"
				fds, errFd := os.ReadDir(fdDir)
				if errFd != nil {
					continue
				}
				for _, fd := range fds {
					fpath := fdDir + "/" + fd.Name()
					f, errOpen := os.Open(fpath)
					if errOpen != nil {
						continue
					}
					var buf [512]byte
					n, _ := f.Read(buf[:])
					f.Close()
					if n > 0 && bytes.Contains(buf[:n], []byte("drm-driver:")) {
						lt.cachedDRMPaths = append(lt.cachedDRMPaths, fpath)
					}
				}
			}
		}
		lt.lastDRMPathsScan = time.Now()
	}

	if len(lt.cachedDRMPaths) == 0 {
		return LiveGPUMetrics{}, false
	}

	// 2. Sample current engine nanoseconds and memory from cached DRM paths
	now := time.Now()
	var totalEngineNS uint64
	var totalMemBytes uint64
	validCount := 0

	validPaths := lt.cachedDRMPaths[:0]
	for _, p := range lt.cachedDRMPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		validPaths = append(validPaths, p)
		validCount++
		lines := bytes.Split(data, []byte("\n"))
		for _, l := range lines {
			if bytes.HasPrefix(l, []byte("drm-engine-")) {
				parts := bytes.Fields(l)
				if len(parts) >= 2 {
					if val, err := strconv.ParseUint(string(parts[1]), 10, 64); err == nil {
						totalEngineNS += val
					}
				}
			} else if bytes.HasPrefix(l, []byte("drm-total-")) {
				parts := bytes.Fields(l)
				if len(parts) >= 2 {
					if val, err := strconv.ParseUint(string(parts[1]), 10, 64); err == nil {
						mult := uint64(1)
						if bytes.Contains(l, []byte("KiB")) {
							mult = 1024
						} else if bytes.Contains(l, []byte("MiB")) {
							mult = 1024 * 1024
						}
						totalMemBytes += val * mult
					}
				}
			}
		}
	}
	lt.cachedDRMPaths = validPaths

	if validCount == 0 {
		lt.lastDRMPathsScan = time.Time{} // Force rescan on next tick
		return LiveGPUMetrics{}, false
	}

	var rawUsage float64
	if !lt.prevDRMTime.IsZero() && totalEngineNS >= lt.prevDRMEngineNS {
		elapsedNS := now.Sub(lt.prevDRMTime).Nanoseconds()
		if elapsedNS > 0 {
			deltaNS := totalEngineNS - lt.prevDRMEngineNS
			rawUsage = (float64(deltaNS) / float64(elapsedNS)) * 100.0
			if rawUsage > 100.0 {
				rawUsage = 100.0
			}
		}
	}
	lt.prevDRMEngineNS = totalEngineNS
	lt.prevDRMTime = now

	if lt.smoothedGPUUsage == 0 && rawUsage > 0 {
		lt.smoothedGPUUsage = rawUsage
	} else {
		// Responsive smoothing (30% previous, 70% current)
		lt.smoothedGPUUsage = lt.smoothedGPUUsage*0.30 + rawUsage*0.70
	}
	if lt.smoothedGPUUsage < 0.5 {
		lt.smoothedGPUUsage = 0
	}

	res := LiveGPUMetrics{
		HasGPU:     true,
		GPUUsage:   lt.smoothedGPUUsage,
		GPUMemUsed: totalMemBytes / (1024 * 1024),
	}
	return res, true
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
	// 1. Direct hwmon inside the DRM card device directory
	if cardName != "" {
		hwmonDir := fmt.Sprintf("/sys/class/drm/%s/device/hwmon", cardName)
		if files, err := os.ReadDir(hwmonDir); err == nil {
			for _, f := range files {
				if strings.HasPrefix(f.Name(), "hwmon") {
					for _, tFile := range []string{"temp1_input", "temp2_input"} {
						tempPath := fmt.Sprintf("%s/%s/%s", hwmonDir, f.Name(), tFile)
						if b, err := os.ReadFile(tempPath); err == nil {
							if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && mVal > 0 {
								return mVal / 1000.0
							}
						}
					}
				}
			}
		}
	}

	// 2. Global /sys/class/hwmon search for GPU-specific sensors
	if files, err := os.ReadDir("/sys/class/hwmon"); err == nil {
		for _, f := range files {
			base := fmt.Sprintf("/sys/class/hwmon/%s", f.Name())
			nameBytes, err := os.ReadFile(filepath.Join(base, "name"))
			if err != nil {
				continue
			}
			sensorName := strings.ToLower(strings.TrimSpace(string(nameBytes)))
			if sensorName == "amdgpu" || sensorName == "radeon" || sensorName == "nouveau" || sensorName == "i915" || sensorName == "xe" {
				for _, tFile := range []string{"temp1_input", "temp2_input"} {
					if b, err := os.ReadFile(filepath.Join(base, tFile)); err == nil {
						if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && mVal > 0 {
							return mVal / 1000.0
						}
					}
				}
			}
		}
	}

	// 3. Fallback for Integrated GPUs (iGPUs) sharing the CPU/SoC thermal package
	if files, err := os.ReadDir("/sys/class/hwmon"); err == nil {
		for _, f := range files {
			base := fmt.Sprintf("/sys/class/hwmon/%s", f.Name())
			nameBytes, err := os.ReadFile(filepath.Join(base, "name"))
			if err != nil {
				continue
			}
			sensorName := strings.ToLower(strings.TrimSpace(string(nameBytes)))
			if sensorName == "coretemp" || sensorName == "k10temp" || sensorName == "zenpower" || sensorName == "cpu_thermal" {
				if b, err := os.ReadFile(filepath.Join(base, "temp1_input")); err == nil {
					if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && mVal > 0 {
						return mVal / 1000.0
					}
				}
			}
		}
	}

	// 4. Linux thermal zone fallback
	if files, err := os.ReadDir("/sys/class/thermal"); err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "thermal_zone") {
				tzPath := fmt.Sprintf("/sys/class/thermal/%s", f.Name())
				typeBytes, _ := os.ReadFile(filepath.Join(tzPath, "type"))
				tzType := strings.ToLower(strings.TrimSpace(string(typeBytes)))
				if strings.Contains(tzType, "gpu") || strings.Contains(tzType, "soc") || strings.Contains(tzType, "pkg") {
					if b, err := os.ReadFile(filepath.Join(tzPath, "temp")); err == nil {
						if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && mVal > 0 {
							if mVal > 1000 {
								return mVal / 1000.0
							}
							return mVal
						}
					}
				}
			}
		}
	}

	return 0
}

func getAMDOrIntelTempFormatted(cardName string) string {
	if cardName != "" {
		hwmonDir := fmt.Sprintf("/sys/class/drm/%s/device/hwmon", cardName)
		if files, err := os.ReadDir(hwmonDir); err == nil {
			for _, f := range files {
				if strings.HasPrefix(f.Name(), "hwmon") {
					base := fmt.Sprintf("%s/%s", hwmonDir, f.Name())
					var edge, junc float64
					if bytes, err := os.ReadFile(fmt.Sprintf("%s/temp1_input", base)); err == nil {
						if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(bytes)), 64); err == nil {
							edge = mVal / 1000.0
						}
					}
					if bytes, err := os.ReadFile(fmt.Sprintf("%s/temp2_input", base)); err == nil {
						if mVal, err := strconv.ParseFloat(strings.TrimSpace(string(bytes)), 64); err == nil {
							junc = mVal / 1000.0
						}
					}
					if edge > 0 {
						if junc > 0 {
							return fmt.Sprintf("%.1f °C (Junction: %.1f °C)", edge, junc)
						}
						return fmt.Sprintf("%.1f °C", edge)
					}
				}
			}
		}
	}

	temp := getAMDOrIntelTemp(cardName)
	if temp > 0 {
		return fmt.Sprintf("%.1f °C", temp)
	}
	return ""
}

func readSysfsFloat(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
}

func readSysfsUint64(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
}

func getIntelGPUUsageAndMemory(cardName string) (float64, uint64, uint64, float64, bool) {
	type freqTriplet struct {
		actPath string
		minPath string
		maxPath string
	}
	triplets := []freqTriplet{
		{
			actPath: fmt.Sprintf("/sys/class/drm/%s/gt/gt0/rps_act_freq_mhz", cardName),
			minPath: fmt.Sprintf("/sys/class/drm/%s/gt/gt0/rps_min_freq_mhz", cardName),
			maxPath: fmt.Sprintf("/sys/class/drm/%s/gt/gt0/rps_max_freq_mhz", cardName),
		},
		{
			actPath: fmt.Sprintf("/sys/class/drm/%s/gt_act_freq_mhz", cardName),
			minPath: fmt.Sprintf("/sys/class/drm/%s/gt_min_freq_mhz", cardName),
			maxPath: fmt.Sprintf("/sys/class/drm/%s/gt_max_freq_mhz", cardName),
		},
		{
			actPath: fmt.Sprintf("/sys/class/drm/%s/gt_cur_freq_mhz", cardName),
			minPath: fmt.Sprintf("/sys/class/drm/%s/gt_min_freq_mhz", cardName),
			maxPath: fmt.Sprintf("/sys/class/drm/%s/gt_max_freq_mhz", cardName),
		},
		{
			actPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/act_freq", cardName),
			minPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/min_freq", cardName),
			maxPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/max_freq", cardName),
		},
		{
			actPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/cur_freq", cardName),
			minPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/min_freq", cardName),
			maxPath: fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0/max_freq", cardName),
		},
	}

	var actFreq, minFreq, maxFreq float64
	foundFreq := false
	for _, tr := range triplets {
		if act, err := readSysfsFloat(tr.actPath); err == nil && act > 0 {
			if max, errMax := readSysfsFloat(tr.maxPath); errMax == nil && max > 0 {
				actFreq = act
				maxFreq = max
				if min, errMin := readSysfsFloat(tr.minPath); errMin == nil && min > 0 {
					minFreq = min
				} else {
					rpnPath := fmt.Sprintf("/sys/class/drm/%s/gt_RPn_freq_mhz", cardName)
					if rpn, errR := readSysfsFloat(rpnPath); errR == nil && rpn > 0 {
						minFreq = rpn
					} else {
						minFreq = 100.0
					}
				}
				foundFreq = true
				break
			}
		}
	}

	if !foundFreq {
		return 0, 0, 0, 0, false
	}

	var usage float64
	if actFreq <= minFreq {
		usage = 0.0
	} else if maxFreq > minFreq {
		usage = ((actFreq - minFreq) / (maxFreq - minFreq)) * 100.0
		if usage > 100.0 {
			usage = 100.0
		}
	}

	var memUsed, memTotal uint64
	var memUsage float64
	lmemUsedPaths := []string{
		fmt.Sprintf("/sys/class/drm/%s/device/lmem_used_bytes", cardName),
		fmt.Sprintf("/sys/class/drm/%s/device/tile0/memory0/used_bytes", cardName),
	}
	lmemTotalPaths := []string{
		fmt.Sprintf("/sys/class/drm/%s/device/lmem_total_bytes", cardName),
		fmt.Sprintf("/sys/class/drm/%s/device/tile0/memory0/total_bytes", cardName),
	}
	for i := range lmemUsedPaths {
		u, errU := readSysfsUint64(lmemUsedPaths[i])
		t, errT := readSysfsUint64(lmemTotalPaths[i])
		if errU == nil && errT == nil && t > 0 {
			memUsed = u / (1024 * 1024)
			memTotal = t / (1024 * 1024)
			memUsage = (float64(u) / float64(t)) * 100.0
			break
		}
	}

	return usage, memUsed, memTotal, memUsage, true
}

func getAMDGPUUsageAndMemory(cardName string) (float64, uint64, uint64, float64, bool) {
	var usage float64
	foundUsage := false

	busyPaths := []string{
		fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", cardName),
		fmt.Sprintf("/sys/class/drm/%s/device/load", cardName),
	}
	for _, bp := range busyPaths {
		if val, err := readSysfsFloat(bp); err == nil {
			usage = val
			if usage > 100.0 {
				usage = 100.0
			}
			foundUsage = true
			break
		}
	}

	if !foundUsage {
		hwmonDir := fmt.Sprintf("/sys/class/drm/%s/device/hwmon", cardName)
		if files, err := os.ReadDir(hwmonDir); err == nil {
			for _, f := range files {
				if strings.HasPrefix(f.Name(), "hwmon") {
					for _, bp := range []string{
						fmt.Sprintf("%s/%s/gpu_busy_percent", hwmonDir, f.Name()),
						fmt.Sprintf("%s/%s/device/gpu_busy_percent", hwmonDir, f.Name()),
					} {
						if val, err := readSysfsFloat(bp); err == nil {
							usage = val
							if usage > 100.0 {
								usage = 100.0
							}
							foundUsage = true
							break
						}
					}
					if foundUsage {
						break
					}
				}
			}
		}
	}

	if !foundUsage {
		sclkPath := fmt.Sprintf("/sys/class/drm/%s/device/pp_dpm_sclk", cardName)
		if b, err := os.ReadFile(sclkPath); err == nil {
			lines := strings.Split(string(b), "\n")
			var minMhz, maxMhz, curMhz float64
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					mhzStr := strings.TrimSuffix(strings.TrimSuffix(parts[1], "Mhz"), "MHz")
					if mhz, err := strconv.ParseFloat(mhzStr, 64); err == nil {
						if minMhz == 0 || mhz < minMhz {
							minMhz = mhz
						}
						if mhz > maxMhz {
							maxMhz = mhz
						}
						if strings.Contains(line, "*") {
							curMhz = mhz
						}
					}
				}
			}
			if curMhz > 0 && maxMhz > minMhz {
				if curMhz <= minMhz {
					usage = 0.0
				} else {
					usage = ((curMhz - minMhz) / (maxMhz - minMhz)) * 100.0
				}
				if usage > 100.0 {
					usage = 100.0
				}
				foundUsage = true
			}
		}
	}

	var memUsed, memTotal uint64
	var memUsage float64

	vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", cardName)
	vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", cardName)
	uVal, errU := readSysfsUint64(vramUsedPath)
	tVal, errT := readSysfsUint64(vramTotalPath)
	if errU == nil && errT == nil && tVal > 0 {
		memUsed = uVal / (1024 * 1024)
		memTotal = tVal / (1024 * 1024)
		memUsage = (float64(uVal) / float64(tVal)) * 100.0
	} else {
		gttUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_gtt_used", cardName)
		gttTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_gtt_total", cardName)
		guVal, gerrU := readSysfsUint64(gttUsedPath)
		gtVal, gerrT := readSysfsUint64(gttTotalPath)
		if gerrU == nil && gerrT == nil && gtVal > 0 {
			memUsed = guVal / (1024 * 1024)
			memTotal = gtVal / (1024 * 1024)
			memUsage = (float64(guVal) / float64(gtVal)) * 100.0
		}
	}

	return usage, memUsed, memTotal, memUsage, foundUsage || memTotal > 0
}

func getNvidiaDRMUsageAndMemory(cardName string) (float64, uint64, uint64, float64, bool) {
	var usage float64
	found := false

	busyPath := fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", cardName)
	if val, err := readSysfsFloat(busyPath); err == nil {
		usage = val
		found = true
	}

	var memUsed, memTotal uint64
	var memUsage float64
	vramUsedPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_used", cardName)
	vramTotalPath := fmt.Sprintf("/sys/class/drm/%s/device/mem_info_vram_total", cardName)
	if uVal, errU := readSysfsUint64(vramUsedPath); errU == nil {
		if tVal, errT := readSysfsUint64(vramTotalPath); errT == nil && tVal > 0 {
			memUsed = uVal / (1024 * 1024)
			memTotal = tVal / (1024 * 1024)
			memUsage = (float64(uVal) / float64(tVal)) * 100.0
			found = true
		}
	}

	return usage, memUsed, memTotal, memUsage, found
}

func getLinuxDevfreqOrArmGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	// 1. Qualcomm Adreno (KGSL)
	kgslBusyPath := "/sys/class/kgsl/kgsl-3d0/gpubusy"
	if b, err := os.ReadFile(kgslBusyPath); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) >= 2 {
			busy, err1 := strconv.ParseFloat(fields[0], 64)
			total, err2 := strconv.ParseFloat(fields[1], 64)
			if err1 == nil && err2 == nil && total > 0 {
				metrics.HasGPU = true
				metrics.GPUUsage = (busy / total) * 100.0
				if metrics.GPUUsage > 100.0 {
					metrics.GPUUsage = 100.0
				}
				metrics.GPUTemp = getAMDOrIntelTemp("")
				return metrics, nil
			}
		}
	}

	// 2. Generic Linux Devfreq subsystem (/sys/class/devfreq/*gpu*, *mali*, *panfrost*, etc.)
	if entries, err := os.ReadDir("/sys/class/devfreq"); err == nil {
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if strings.Contains(name, "gpu") || strings.Contains(name, "mali") ||
				strings.Contains(name, "panfrost") || strings.Contains(name, "v3d") ||
				strings.Contains(name, "kgsl") || strings.Contains(name, "lima") {
				base := fmt.Sprintf("/sys/class/devfreq/%s", e.Name())

				if loadBytes, err := os.ReadFile(fmt.Sprintf("%s/load", base)); err == nil {
					loadStr := strings.TrimSpace(string(loadBytes))
					if strings.Contains(loadStr, "@") {
						parts := strings.Split(loadStr, "@")
						if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
							metrics.HasGPU = true
							metrics.GPUUsage = val
							metrics.GPUTemp = getAMDOrIntelTemp("")
							return metrics, nil
						}
					} else {
						fields := strings.Fields(loadStr)
						if len(fields) >= 2 {
							busy, err1 := strconv.ParseFloat(fields[0], 64)
							total, err2 := strconv.ParseFloat(fields[1], 64)
							if err1 == nil && err2 == nil && total > 0 {
								metrics.HasGPU = true
								metrics.GPUUsage = (busy / total) * 100.0
								metrics.GPUTemp = getAMDOrIntelTemp("")
								return metrics, nil
							}
						}
					}
				}

				cur, err1 := readSysfsFloat(fmt.Sprintf("%s/cur_freq", base))
				min, err2 := readSysfsFloat(fmt.Sprintf("%s/min_freq", base))
				max, err3 := readSysfsFloat(fmt.Sprintf("%s/max_freq", base))
				if err1 == nil && err2 == nil && err3 == nil && max > min {
					metrics.HasGPU = true
					if cur <= min {
						metrics.GPUUsage = 0.0
					} else {
						metrics.GPUUsage = ((cur - min) / (max - min)) * 100.0
					}
					metrics.GPUTemp = getAMDOrIntelTemp("")
					return metrics, nil
				}
			}
		}
	}

	// 3. ARM Mali legacy driver (/sys/class/misc/mali0/device/utilization)
	maliUtilPath := "/sys/class/misc/mali0/device/utilization"
	if b, err := os.ReadFile(maliUtilPath); err == nil {
		if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
			metrics.HasGPU = true
			if val <= 100.0 {
				metrics.GPUUsage = val
			} else {
				metrics.GPUUsage = (val / 255.0) * 100.0
			}
			metrics.GPUTemp = getAMDOrIntelTemp("")
			return metrics, nil
		}
	}

	// 4. Raspberry Pi vcgencmd
	if _, err := exec.LookPath("vcgencmd"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "vcgencmd", "measure_clock", "core")
		if out, err := cmd.Output(); err == nil {
			parts := strings.Split(strings.TrimSpace(string(out)), "=")
			if len(parts) >= 2 {
				if hz, err := strconv.ParseFloat(parts[1], 64); err == nil && hz > 0 {
					metrics.HasGPU = true
					minHz := 250000000.0
					maxHz := 500000000.0
					if hz <= minHz {
						metrics.GPUUsage = 0.0
					} else if hz >= maxHz {
						metrics.GPUUsage = 100.0
					} else {
						metrics.GPUUsage = ((hz - minHz) / (maxHz - minHz)) * 100.0
					}

					cmdTemp := exec.CommandContext(ctx, "vcgencmd", "measure_temp")
					if tOut, err := cmdTemp.Output(); err == nil {
						tParts := strings.Split(strings.TrimSpace(string(tOut)), "=")
						if len(tParts) >= 2 {
							raw := strings.TrimSuffix(strings.TrimSuffix(tParts[1], "'C"), "C")
							if tVal, err := strconv.ParseFloat(raw, 64); err == nil {
								metrics.GPUTemp = tVal
							}
						}
					}
					return metrics, nil
				}
			}
		}
	}

	return metrics, fmt.Errorf("no ARM/devfreq GPU telemetry found")
}

func getMacOSGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ioreg", "-r", "-c", "IOAccelerator", "-d", "2")
	out, err := cmd.Output()
	if err != nil {
		return metrics, err
	}

	lines := strings.Split(string(out), "\n")
	foundUtil := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `"Device Utilization %"`) || strings.Contains(line, `"GPU Activity(%)"`) {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				cleanVal := strings.Trim(strings.TrimSpace(parts[1]), ",;")
				if val, err := strconv.ParseFloat(cleanVal, 64); err == nil {
					metrics.HasGPU = true
					metrics.GPUUsage = val
					if metrics.GPUUsage > 100.0 {
						metrics.GPUUsage = 100.0
					}
					foundUtil = true
				}
			}
		}

		if strings.Contains(line, `"In use system memory"`) || strings.Contains(line, `"Allocated memory"`) {
			parts := strings.Split(line, "=")
			if len(parts) >= 2 {
				cleanVal := strings.Trim(strings.TrimSpace(parts[1]), ",;")
				if bytesVal, err := strconv.ParseUint(cleanVal, 10, 64); err == nil && bytesVal > 0 {
					metrics.GPUMemUsed = bytesVal / (1024 * 1024)
				}
			}
		}
	}

	if foundUtil || metrics.HasGPU {
		return metrics, nil
	}

	return metrics, fmt.Errorf("no macOS GPU telemetry parsed")
}

func getWindowsGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	// 1. Try Windows typeperf for GPU 3D engine counters
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "typeperf", `\GPU Engine(*engtype_3D)\Utilization Percentage`, "-sc", "1")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) >= 2 {
			dataLine := lines[len(lines)-1]
			records := strings.Split(dataLine, ",")
			var totalUtil float64
			validCount := 0
			for i := 1; i < len(records); i++ {
				raw := strings.Trim(strings.TrimSpace(records[i]), `"`)
				if val, err := strconv.ParseFloat(raw, 64); err == nil {
					totalUtil += val
					validCount++
				}
			}
			if validCount > 0 {
				metrics.HasGPU = true
				if totalUtil > 100.0 {
					totalUtil = 100.0
				}
				metrics.GPUUsage = totalUtil
				return metrics, nil
			}
		}
	}

	// 2. PowerShell WMI fallback
	ctxPS, cancelPS := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancelPS()

	psCmd := exec.CommandContext(ctxPS, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`$u = (Get-CimInstance Win32_PerfFormattedData_GPUPerformanceCounters_GPUEngine | Where-Object {$_.Name -like '*engtype_3D'} | Measure-Object -Property UtilizationPercentage -Sum).Sum; if ($u -ne $null) { [math]::Round($u, 1) }`)
	psOut, psErr := psCmd.Output()
	if psErr == nil {
		valStr := strings.TrimSpace(string(psOut))
		if val, err := strconv.ParseFloat(valStr, 64); err == nil {
			metrics.HasGPU = true
			if val > 100.0 {
				val = 100.0
			}
			metrics.GPUUsage = val
			return metrics, nil
		}
	}

	return metrics, fmt.Errorf("no Windows GPU telemetry found")
}

func getGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	// 1. Try Nvidia first (nvidia-smi works on Linux & Windows)
	if nvidia, err := getNvidiaGPUMetrics(); err == nil && nvidia.HasGPU {
		return nvidia, nil
	}

	// 2. macOS Platform (Apple Silicon M1/M2/M3/M4 & Intel Macs)
	if runtime.GOOS == "darwin" {
		if macMetrics, err := getMacOSGPUMetrics(); err == nil && macMetrics.HasGPU {
			return macMetrics, nil
		}
	}

	// 3. Windows Platform (Intel, AMD, Qualcomm, etc. via DirectX/WMI performance counters)
	if runtime.GOOS == "windows" {
		if winMetrics, err := getWindowsGPUMetrics(); err == nil && winMetrics.HasGPU {
			return winMetrics, nil
		}
	}

	// 4. Linux Platform (DRM cards: Intel, AMD, Nvidia Nouveau, etc.)
	if runtime.GOOS == "linux" {
		files, err := os.ReadDir("/sys/class/drm")
		if err == nil {
			var candidates []LiveGPUMetrics
			for _, file := range files {
				cardName := file.Name()
				if !strings.HasPrefix(cardName, "card") || strings.Contains(cardName, "-") {
					continue
				}

				devicePath := fmt.Sprintf("/sys/class/drm/%s/device", cardName)
				if _, err := os.Stat(devicePath); err != nil {
					continue
				}

				vendorBytes, _ := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/device/vendor", cardName))
				vendorID := strings.TrimSpace(strings.ToLower(string(vendorBytes)))

				driverLink, _ := os.Readlink(fmt.Sprintf("/sys/class/drm/%s/device/driver", cardName))
				driverName := strings.ToLower(filepath.Base(driverLink))

				// A. AMD GPU (Vendor ID: 0x1002, or driver amdgpu/radeon)
				if strings.Contains(vendorID, "1002") || driverName == "amdgpu" || driverName == "radeon" {
					usage, memUsed, memTotal, memUsage, ok := getAMDGPUUsageAndMemory(cardName)
					if ok {
						c := LiveGPUMetrics{
							HasGPU:      true,
							GPUUsage:    usage,
							GPUMemUsed:  memUsed,
							GPUMemTotal: memTotal,
							GPUMemUsage: memUsage,
							GPUTemp:     getAMDOrIntelTemp(cardName),
						}
						candidates = append(candidates, c)
						continue
					}
				}

				// B. Intel GPU (Vendor ID: 0x8086, or driver i915/xe)
				if strings.Contains(vendorID, "8086") || driverName == "i915" || driverName == "xe" {
					usage, memUsed, memTotal, memUsage, ok := getIntelGPUUsageAndMemory(cardName)
					if ok {
						c := LiveGPUMetrics{
							HasGPU:      true,
							GPUUsage:    usage,
							GPUMemUsed:  memUsed,
							GPUMemTotal: memTotal,
							GPUMemUsage: memUsage,
							GPUTemp:     getAMDOrIntelTemp(cardName),
						}
						candidates = append(candidates, c)
						continue
					}
				}

				// C. Nvidia GPU via Nouveau / open-source (Vendor ID: 0x10de, or driver nouveau)
				if strings.Contains(vendorID, "10de") || driverName == "nouveau" {
					usage, memUsed, memTotal, memUsage, ok := getNvidiaDRMUsageAndMemory(cardName)
					if ok {
						c := LiveGPUMetrics{
							HasGPU:      true,
							GPUUsage:    usage,
							GPUMemUsed:  memUsed,
							GPUMemTotal: memTotal,
							GPUMemUsage: memUsage,
							GPUTemp:     getAMDOrIntelTemp(cardName),
						}
						candidates = append(candidates, c)
						continue
					}
				}

				// D. Any other DRM GPU (e.g. Broadcom v3d/vc4, ARM Panfrost, Lima, etc.)
				busyPath := fmt.Sprintf("/sys/class/drm/%s/device/gpu_busy_percent", cardName)
				if val, err := readSysfsFloat(busyPath); err == nil {
					c := LiveGPUMetrics{
						HasGPU:   true,
						GPUUsage: val,
						GPUTemp:  getAMDOrIntelTemp(cardName),
					}
					if c.GPUUsage > 100.0 {
						c.GPUUsage = 100.0
					}
					candidates = append(candidates, c)
					continue
				}
			}

			// If multiple GPUs found (e.g. discrete AMD/Intel Arc + integrated GPU),
			// pick discrete (GPUMemTotal > 0) or active (GPUUsage > 0), else first candidate.
			if len(candidates) > 0 {
				best := candidates[0]
				for _, c := range candidates[1:] {
					if (c.GPUMemTotal > 0 && best.GPUMemTotal == 0) || (c.GPUUsage > best.GPUUsage) {
						best = c
					}
				}
				return best, nil
			}
		}

		// 5. Linux ARM / Devfreq / SoC GPU fallback (RPi, Adreno, Mali, Rockchip)
		if armMetrics, err := getLinuxDevfreqOrArmGPUMetrics(); err == nil && armMetrics.HasGPU {
			return armMetrics, nil
		}
	}

	return metrics, fmt.Errorf("no GPU telemetry found")
}

func getNvidiaGPUMetrics() (LiveGPUMetrics, error) {
	metrics := LiveGPUMetrics{}

	nvsmiPath := "nvidia-smi"
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("nvidia-smi"); err != nil {
			winPaths := []string{
				`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
				`C:\Windows\System32\nvidia-smi.exe`,
			}
			found := false
			for _, wp := range winPaths {
				if _, err := os.Stat(wp); err == nil {
					nvsmiPath = wp
					found = true
					break
				}
			}
			if !found {
				return metrics, fmt.Errorf("nvidia-smi not found")
			}
		}
	} else {
		if _, err := exec.LookPath("nvidia-smi"); err != nil {
			return metrics, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, nvsmiPath, "--query-gpu=utilization.gpu,utilization.memory,temperature.gpu,memory.used,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return metrics, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 5 {
			continue
		}

		c0 := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%"))
		c1 := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "%"))
		c2 := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[2]), "%"))
		c3 := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[3]), "%"))
		c4 := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[4]), "%"))

		gpuUtil, err1 := strconv.ParseFloat(c0, 64)
		memUtil, err2 := strconv.ParseFloat(c1, 64)
		temp, err3 := strconv.ParseFloat(c2, 64)
		memUsed, err4 := strconv.ParseUint(c3, 10, 64)
		memTotal, err5 := strconv.ParseUint(c4, 10, 64)

		if err1 == nil {
			metrics.HasGPU = true
			if gpuUtil > metrics.GPUUsage || !metrics.HasGPU {
				metrics.GPUUsage = gpuUtil
			}
			if err2 == nil {
				metrics.GPUMemUsage = memUtil
			}
			if err3 == nil && temp > 0 {
				metrics.GPUTemp = temp
			}
			if err4 == nil {
				metrics.GPUMemUsed = memUsed
			}
			if err5 == nil {
				metrics.GPUMemTotal = memTotal
			}
		}
	}

	if metrics.HasGPU {
		return metrics, nil
	}

	return metrics, fmt.Errorf("failed to parse GPU metrics")
}

// GPUDetails holds comprehensive details about graphics hardware, memory, clocks, displays, and APIs
type GPUDetails struct {
	Name           string
	Vendor         string
	Driver         string
	DriverVersion  string
	DeviceID       string
	Subsystem      string
	PCIBus         string
	VRAMTotal      string
	VRAMUsed       string
	VRAMFree       string
	VRAMType       string
	Temperature    string
	CurrentClock   string
	PowerState     string
	ActiveDisplays string
	OpenGL         string
	OpenGLRenderer string
	OpenGLCore     string
	OpenGLES       string
	Vulkan         string
	VulkanDriver   string
	OpenCL         string
	CUDA           string
	Metal          string
	DirectX        string
}

// getSubsystemVendorName maps PCI subsystem vendor hex IDs to human-readable manufacturer names
func getSubsystemVendorName(hex string) string {
	hex = strings.ToLower(strings.TrimPrefix(hex, "0x"))
	switch hex {
	case "1043":
		return "ASUS"
	case "1028":
		return "Dell"
	case "103c":
		return "HP"
	case "17aa":
		return "Lenovo"
	case "1462":
		return "MSI"
	case "1458":
		return "Gigabyte"
	case "1da2", "174b":
		return "Sapphire"
	case "1787":
		return "PowerColor"
	case "1849":
		return "ASRock"
	case "1682":
		return "XFX"
	case "19da", "1b4b":
		return "Zotac"
	case "196e":
		return "PNY"
	case "1569":
		return "Palit"
	case "10b0":
		return "Gainward"
	case "10de":
		return "Nvidia"
	case "1002":
		return "AMD"
	case "8086":
		return "Intel"
	case "1025":
		return "Acer"
	case "1558":
		return "Clevo"
	case "144d":
		return "Samsung"
	case "106b":
		return "Apple"
	case "1d05":
		return "Tongfang"
	case "1af4":
		return "VirtIO"
	case "15ad":
		return "VMware"
	case "80ee":
		return "VirtualBox"
	case "1414":
		return "Microsoft"
	}
	return ""
}

// getNvidiaDetails queries nvidia-smi cross-platform (Linux & Windows) for deep telemetry
func getNvidiaDetails() (name, driverVer, pciBus, vramTotal, vramUsed, vramFree, temp, power, clock, displays string, ok bool) {
	nvsmiPath := "nvidia-smi"
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("nvidia-smi"); err == nil {
			nvsmiPath = p
		} else if _, err := os.Stat(`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`); err == nil {
			nvsmiPath = `C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`
		} else {
			return
		}
	} else {
		if _, err := exec.LookPath("nvidia-smi"); err != nil {
			return
		}
	}

	out := runCommand(nvsmiPath, "--query-gpu=gpu_name,driver_version,pci.bus_id,memory.total,memory.used,memory.free,temperature.gpu,power.draw,power.limit,clocks.current.graphics,clocks.max.graphics,display_active", "--format=csv,noheader,nounits")
	if out == "" {
		return
	}

	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		return
	}
	line := strings.TrimSpace(lines[0])
	parts := strings.Split(line, ",")
	if len(parts) < 11 {
		return
	}

	name = strings.TrimSpace(parts[0])
	driverVer = strings.TrimSpace(parts[1])
	pciBus = strings.TrimSpace(parts[2])

	totMB, _ := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 64)
	usedMB, _ := strconv.ParseUint(strings.TrimSpace(parts[4]), 10, 64)
	freeMB, _ := strconv.ParseUint(strings.TrimSpace(parts[5]), 10, 64)
	if totMB > 0 {
		vramTotal = FormatBytes(totMB * 1024 * 1024)
		vramUsed = FormatBytes(usedMB * 1024 * 1024)
		vramFree = FormatBytes(freeMB * 1024 * 1024)
	}

	if t, err := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64); err == nil && t > 0 {
		temp = fmt.Sprintf("%.1f °C", t)
	}

	drawW, _ := strconv.ParseFloat(strings.TrimSpace(parts[7]), 64)
	limitW, _ := strconv.ParseFloat(strings.TrimSpace(parts[8]), 64)
	if drawW > 0 {
		if limitW > 0 {
			power = fmt.Sprintf("D0 (%.1f W / %.1f W)", drawW, limitW)
		} else {
			power = fmt.Sprintf("D0 (%.1f W)", drawW)
		}
	}

	curC := strings.TrimSpace(parts[9])
	maxC := strings.TrimSpace(parts[10])
	if curC != "" && curC != "[Not Supported]" {
		if maxC != "" && maxC != "[Not Supported]" {
			clock = fmt.Sprintf("%s MHz (Max: %s MHz)", curC, maxC)
		} else {
			clock = fmt.Sprintf("%s MHz", curC)
		}
	}

	if len(parts) >= 12 {
		disp := strings.TrimSpace(parts[11])
		if disp == "Enabled" || disp == "1" {
			displays = "Connected (Active)"
		}
	}

	ok = true
	return
}

// getWindowsGPUDetails queries Windows WMI/CIM for full display adapter telemetry
func getWindowsGPUDetails(details *GPUDetails) {
	psCmd := `Get-CimInstance Win32_VideoController | Select-Object Name,AdapterRAM,DriverVersion,VideoProcessor,Status,CurrentHorizontalResolution,CurrentVerticalResolution,CurrentRefreshRate,PNPDeviceID | ConvertTo-Json -Compress`
	out := runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)

	type winGPU struct {
		Name                        string      `json:"Name"`
		AdapterRAM                  interface{} `json:"AdapterRAM"`
		DriverVersion               string      `json:"DriverVersion"`
		VideoProcessor              string      `json:"VideoProcessor"`
		Status                      string      `json:"Status"`
		CurrentHorizontalResolution int         `json:"CurrentHorizontalResolution"`
		CurrentVerticalResolution   int         `json:"CurrentVerticalResolution"`
		CurrentRefreshRate          int         `json:"CurrentRefreshRate"`
		PNPDeviceID                 string      `json:"PNPDeviceID"`
	}

	var gpus []winGPU
	if out != "" {
		trimmed := strings.TrimSpace(out)
		if strings.HasPrefix(trimmed, "[") {
			_ = json.Unmarshal([]byte(trimmed), &gpus)
		} else if strings.HasPrefix(trimmed, "{") {
			var single winGPU
			if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
				gpus = append(gpus, single)
			}
		}
	}

	if len(gpus) == 0 {
		wmicOut := runCommand("wmic", "path", "win32_VideoController", "get", "Name,AdapterRAM,DriverVersion,Status,PNPDeviceID", "/format:list")
		if wmicOut != "" {
			var g winGPU
			for _, line := range strings.Split(wmicOut, "\n") {
				parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(parts) == 2 {
					k, v := parts[0], parts[1]
					switch k {
					case "Name":
						g.Name = v
					case "AdapterRAM":
						g.AdapterRAM = v
					case "DriverVersion":
						g.DriverVersion = v
					case "Status":
						g.Status = v
					case "PNPDeviceID":
						g.PNPDeviceID = v
					}
				}
			}
			if g.Name != "" {
				gpus = append(gpus, g)
			}
		}
	}

	if len(gpus) > 0 {
		g := gpus[0]
		if details.Name == "" || details.Name == "Unknown" {
			details.Name = g.Name
		}
		if details.DriverVersion == "" {
			details.DriverVersion = g.DriverVersion
		}
		if g.Status != "" && details.PowerState == "" {
			details.PowerState = fmt.Sprintf("%s (Active)", g.Status)
		}

		if details.VRAMTotal == "" {
			var ramBytes uint64
			switch v := g.AdapterRAM.(type) {
			case float64:
				ramBytes = uint64(v)
			case string:
				ramBytes, _ = strconv.ParseUint(v, 10, 64)
			}
			if ramBytes > 0 {
				details.VRAMTotal = FormatBytes(ramBytes)
			}
		}

		if g.CurrentHorizontalResolution > 0 && g.CurrentVerticalResolution > 0 {
			if g.CurrentRefreshRate > 0 {
				details.ActiveDisplays = fmt.Sprintf("%dx%d @ %dHz (Connected)", g.CurrentHorizontalResolution, g.CurrentVerticalResolution, g.CurrentRefreshRate)
			} else {
				details.ActiveDisplays = fmt.Sprintf("%dx%d (Connected)", g.CurrentHorizontalResolution, g.CurrentVerticalResolution)
			}
		}

		if g.PNPDeviceID != "" {
			upper := strings.ToUpper(g.PNPDeviceID)
			if venIdx := strings.Index(upper, "VEN_"); venIdx != -1 && len(upper) >= venIdx+8 {
				venHex := strings.ToLower(upper[venIdx+4 : venIdx+8])
				switch venHex {
				case "10de":
					details.Vendor = "Nvidia"
				case "1002":
					details.Vendor = "AMD"
				case "8086":
					details.Vendor = "Intel"
				case "17cb":
					details.Vendor = "Qualcomm"
				}
				if devIdx := strings.Index(upper, "DEV_"); devIdx != -1 && len(upper) >= devIdx+8 {
					devHex := strings.ToLower(upper[devIdx+4 : devIdx+8])
					details.DeviceID = fmt.Sprintf("[%s:%s]", venHex, devHex)
				}
			}
		}
	}
}

// getDarwinGPUDetails queries macOS SPDisplaysDataType for deep Apple Silicon & Intel Mac GPU telemetry
func getDarwinGPUDetails(details *GPUDetails) {
	out := runCommand("system_profiler", "SPDisplaysDataType")
	if out == "" {
		return
	}

	lines := strings.Split(out, "\n")
	var currentDisplay string
	var inDisplays bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Chipset Model:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				details.Name = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(trimmed, "Vendor:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				vLower := strings.ToLower(v)
				if strings.Contains(vLower, "apple") {
					details.Vendor = "Apple"
					details.VRAMType = "Unified System Memory"
				} else if strings.Contains(vLower, "intel") {
					details.Vendor = "Intel"
				} else if strings.Contains(vLower, "amd") || strings.Contains(vLower, "ati") {
					details.Vendor = "AMD"
				} else if strings.Contains(vLower, "nvidia") {
					details.Vendor = "Nvidia"
				} else {
					details.Vendor = v
				}
			}
		} else if strings.HasPrefix(trimmed, "Bus:") || strings.HasPrefix(trimmed, "PCIe Lane Width:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 && details.PCIBus == "" {
				details.PCIBus = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(trimmed, "Total Number of Cores:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				details.Subsystem = fmt.Sprintf("%s GPU Cores", strings.TrimSpace(parts[1]))
			}
		} else if strings.HasPrefix(trimmed, "VRAM (Total):") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				details.VRAMTotal = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(trimmed, "Metal Support:") || strings.HasPrefix(trimmed, "Metal:") || strings.HasPrefix(trimmed, "Metal Family:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				details.Metal = strings.TrimSpace(parts[1])
			}
		} else if trimmed == "Displays:" {
			inDisplays = true
		} else if inDisplays {
			if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, "Type:") && !strings.Contains(trimmed, "Resolution:") {
				dName := strings.TrimSuffix(trimmed, ":")
				if dName != "" {
					currentDisplay = dName
				}
			} else if strings.HasPrefix(trimmed, "Resolution:") {
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					res := strings.TrimSpace(parts[1])
					dispStr := res
					if currentDisplay != "" {
						dispStr = fmt.Sprintf("%s (%s, Connected)", currentDisplay, res)
					} else {
						dispStr = fmt.Sprintf("%s (Connected)", res)
					}
					if details.ActiveDisplays == "" {
						details.ActiveDisplays = dispStr
					} else {
						details.ActiveDisplays += ", " + dispStr
					}
				}
			}
		}
	}
}

// getBSDGPUDetails queries FreeBSD / OpenBSD pciconf for GPU and display controllers
func getBSDGPUDetails(details *GPUDetails) {
	out := runCommand("pciconf", "-lv")
	if out == "" {
		return
	}
	var inVGA bool
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "class=0x03") || strings.Contains(line, "subclass=VGA") || strings.Contains(line, "class=display") {
			inVGA = true
		} else if inVGA && strings.Contains(line, "class=0x") {
			inVGA = false
		}
		if inVGA {
			if strings.HasPrefix(trimmed, "vendor") && strings.Contains(trimmed, "=") {
				parts := strings.SplitN(trimmed, "=", 2)
				v := strings.Trim(strings.TrimSpace(parts[1]), "'")
				details.Vendor = v
			} else if strings.HasPrefix(trimmed, "device") && strings.Contains(trimmed, "=") {
				parts := strings.SplitN(trimmed, "=", 2)
				d := strings.Trim(strings.TrimSpace(parts[1]), "'")
				details.Name = d
			}
		}
	}
}

// GetGPUDetails collects detailed GPU and graphics API information
func GetGPUDetails() *GPUDetails {
	details := &GPUDetails{}

	// 1. Resolve GPU Name
	if name := getLinuxGPU(); name != "" {
		details.Name = name
	} else if name := getGPUInfo(); name != "" && name != "Unknown" {
		details.Name = name
	}

	// 2. Resolve Graphics Libraries
	glVer, glRenderer, glCore, glES, glVram, glUnified := getOpenGLDetails()
	details.OpenGL = glVer
	details.OpenGLRenderer = glRenderer
	details.OpenGLCore = glCore
	details.OpenGLES = glES
	if glVram != "" && details.VRAMTotal == "" {
		details.VRAMTotal = glVram
		if glUnified {
			details.VRAMType = "Unified System Memory"
		}
	}

	vendorHint := details.Vendor
	if vendorHint == "" {
		vendorHint = details.Name
	}
	details.Vulkan, details.VulkanDriver = getVulkanDetails(vendorHint)
	details.OpenCL = getOpenCLDetails()
	details.CUDA = getCUDAVersion()

	// 3. Resolve Hardware, Vendor, Driver, VRAM, Clocks, Displays based on OS
	if runtime.GOOS == "linux" {
		// Check nvidia-smi first (Nvidia Proprietary driver)
		if nName, nVer, nBus, nTot, nUsed, nFree, nTemp, nPwr, nClk, nDisp, ok := getNvidiaDetails(); ok {
			if nName != "" {
				details.Name = nName
			}
			details.Vendor = "Nvidia"
			details.Driver = "Nvidia Proprietary"
			if nVer != "" {
				details.DriverVersion = nVer
			}
			if nBus != "" {
				details.PCIBus = nBus
			}
			if nTot != "" {
				details.VRAMTotal = nTot
				details.VRAMUsed = nUsed
				details.VRAMFree = nFree
			}
			if nTemp != "" {
				details.Temperature = nTemp
			}
			if nPwr != "" {
				details.PowerState = nPwr
			}
			if nClk != "" {
				details.CurrentClock = nClk
			}
			if nDisp != "" && details.ActiveDisplays == "" {
				details.ActiveDisplays = nDisp
			}
		}

		files, err := os.ReadDir("/sys/class/drm")
		if err == nil {
			var bestCard string
			var hasDisplay bool

			// Find active display card or boot VGA card
			for _, file := range files {
				cardName := file.Name()
				if !strings.HasPrefix(cardName, "card") || strings.Contains(cardName, "-") {
					continue
				}
				devicePath := fmt.Sprintf("/sys/class/drm/%s/device", cardName)
				if _, err := os.Stat(devicePath); err != nil {
					continue
				}

				if bestCard == "" {
					bestCard = cardName
				}

				// Check if boot VGA
				bootVgaBytes, _ := os.ReadFile(fmt.Sprintf("%s/boot_vga", devicePath))
				if strings.TrimSpace(string(bootVgaBytes)) == "1" {
					bestCard = cardName
					hasDisplay = true
					break
				}

				// Check if card has a connected display
				for _, subF := range files {
					if strings.HasPrefix(subF.Name(), cardName+"-") {
						statusBytes, errSt := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/status", subF.Name()))
						if errSt == nil && strings.TrimSpace(string(statusBytes)) == "connected" {
							bestCard = cardName
							hasDisplay = true
							break
						}
					}
				}
				if hasDisplay {
					break
				}
			}

			if bestCard != "" {
				cardPath := fmt.Sprintf("/sys/class/drm/%s", bestCard)
				devicePath := fmt.Sprintf("%s/device", cardPath)

				// PCI Bus
				if details.PCIBus == "" {
					if target, err := os.Readlink(devicePath); err == nil {
						parts := strings.Split(target, "/")
						details.PCIBus = parts[len(parts)-1]
					}
				}

				// Resolve Driver
				if details.Driver == "" {
					driverLink := fmt.Sprintf("%s/driver", devicePath)
					if target, err := os.Readlink(driverLink); err == nil {
						parts := strings.Split(target, "/")
						details.Driver = parts[len(parts)-1]
					}
				}

				// Resolve Vendor & Device ID
				vendorBytes, errV := os.ReadFile(fmt.Sprintf("%s/vendor", devicePath))
				devIDBytes, errD := os.ReadFile(fmt.Sprintf("%s/device", devicePath))
				var vendorHex, devHex string
				if errV == nil {
					vendorHex = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(string(vendorBytes))), "0x")
					if details.Vendor == "" {
						if vendorHex == "8086" {
							details.Vendor = "Intel"
						} else if vendorHex == "10de" {
							details.Vendor = "Nvidia"
						} else if vendorHex == "1002" {
							details.Vendor = "AMD"
						} else if vendorHex == "17cb" {
							details.Vendor = "Qualcomm"
						} else if vendorHex == "13d3" {
							details.Vendor = "IMC Networks"
						}
					}
				}
				if errD == nil {
					devHex = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(string(devIDBytes))), "0x")
				}
				if vendorHex != "" && devHex != "" && details.DeviceID == "" {
					details.DeviceID = fmt.Sprintf("[%s:%s]", vendorHex, devHex)
				}

				// Subsystem vendor/device
				if details.Subsystem == "" {
					subVendorBytes, _ := os.ReadFile(fmt.Sprintf("%s/subsystem_vendor", devicePath))
					subDevBytes, _ := os.ReadFile(fmt.Sprintf("%s/subsystem_device", devicePath))
					subVHex := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(string(subVendorBytes))), "0x")
					subDHex := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(string(subDevBytes))), "0x")
					if subVHex != "" {
						subName := getSubsystemVendorName(subVHex)
						if subName != "" {
							if subDHex != "" {
								details.Subsystem = fmt.Sprintf("%s [%s:%s]", subName, subVHex, subDHex)
							} else {
								details.Subsystem = fmt.Sprintf("%s [%s]", subName, subVHex)
							}
						} else if subDHex != "" {
							details.Subsystem = fmt.Sprintf("[%s:%s]", subVHex, subDHex)
						}
					}
				}

				// Power State
				if details.PowerState == "" {
					powerBytes, errP := os.ReadFile(fmt.Sprintf("%s/power_state", devicePath))
					if errP == nil {
						pState := strings.TrimSpace(string(powerBytes))
						if pState != "" {
							details.PowerState = pState
						}
					}
				}

				// Clocks
				if details.CurrentClock == "" {
					// 1. AMD pp_dpm_sclk
					sclkPath := fmt.Sprintf("%s/pp_dpm_sclk", devicePath)
					if sclkBytes, errS := os.ReadFile(sclkPath); errS == nil {
						sLines := strings.Split(strings.TrimSpace(string(sclkBytes)), "\n")
						var curClk, minClk, maxClk string
						if len(sLines) > 0 {
							parts0 := strings.Split(sLines[0], ":")
							if len(parts0) > 1 {
								minClk = strings.TrimSpace(parts0[1])
							}
							partsLast := strings.Split(sLines[len(sLines)-1], ":")
							if len(partsLast) > 1 {
								maxClk = strings.TrimSpace(strings.TrimSuffix(partsLast[1], "*"))
							}
							for _, sl := range sLines {
								if strings.Contains(sl, "*") {
									parts := strings.Split(sl, ":")
									if len(parts) > 1 {
										curClk = strings.TrimSpace(strings.TrimSuffix(parts[1], "*"))
									}
								}
							}
						}
						if curClk != "" {
							if minClk != "" && maxClk != "" && minClk != maxClk {
								details.CurrentClock = fmt.Sprintf("%s (Min: %s │ Max: %s)", curClk, minClk, maxClk)
							} else {
								details.CurrentClock = curClk
							}
						}
					}

					// 2. Intel iGPU / Arc / Xe sysfs
					if details.CurrentClock == "" {
						var curFreq, maxFreq, minFreq []byte
						curFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt/gt0/rps_act_freq_mhz", devicePath, bestCard))
						if len(curFreq) == 0 {
							curFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt_act_freq_mhz", devicePath, bestCard))
						}
						if len(curFreq) == 0 {
							curFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt_cur_freq_mhz", devicePath, bestCard))
						}
						if len(curFreq) == 0 {
							curFreq, _ = os.ReadFile(fmt.Sprintf("%s/tile0/gt0/freq0/act_freq", devicePath))
						}

						maxFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt/gt0/rps_max_freq_mhz", devicePath, bestCard))
						if len(maxFreq) == 0 {
							maxFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt_max_freq_mhz", devicePath, bestCard))
						}
						minFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt/gt0/rps_min_freq_mhz", devicePath, bestCard))
						if len(minFreq) == 0 {
							minFreq, _ = os.ReadFile(fmt.Sprintf("%s/drm/%s/gt_min_freq_mhz", devicePath, bestCard))
						}

						cStr := strings.TrimSpace(string(curFreq))
						maxStr := strings.TrimSpace(string(maxFreq))
						minStr := strings.TrimSpace(string(minFreq))
						if cStr != "" {
							if maxStr != "" && minStr != "" {
								details.CurrentClock = fmt.Sprintf("%s MHz (Min: %s MHz │ Max: %s MHz)", cStr, minStr, maxStr)
							} else if maxStr != "" {
								details.CurrentClock = fmt.Sprintf("%s MHz (Max: %s MHz)", cStr, maxStr)
							} else {
								details.CurrentClock = fmt.Sprintf("%s MHz", cStr)
							}
						}
					}

					// 3. Fallback: hwmon freq1_input
					if details.CurrentClock == "" {
						hwmonDir := fmt.Sprintf("%s/hwmon", devicePath)
						if hwFiles, errH := os.ReadDir(hwmonDir); errH == nil {
							for _, hf := range hwFiles {
								if strings.HasPrefix(hf.Name(), "hwmon") {
									freqBytes, errF := os.ReadFile(fmt.Sprintf("%s/%s/freq1_input", hwmonDir, hf.Name()))
									if errF == nil {
										if fVal, errP := strconv.ParseFloat(strings.TrimSpace(string(freqBytes)), 64); errP == nil && fVal > 0 {
											details.CurrentClock = fmt.Sprintf("%.0f MHz", fVal/1000000.0)
											break
										}
									}
								}
							}
						}
					}
				}

				// Power consumption (AMD & Intel hwmon)
				hwmonDir := fmt.Sprintf("%s/hwmon", devicePath)
				if hwFiles, errH := os.ReadDir(hwmonDir); errH == nil {
					for _, hf := range hwFiles {
						if strings.HasPrefix(hf.Name(), "hwmon") {
							base := fmt.Sprintf("%s/%s", hwmonDir, hf.Name())
							pBytes, errP := os.ReadFile(fmt.Sprintf("%s/power1_average", base))
							if errP != nil {
								pBytes, _ = os.ReadFile(fmt.Sprintf("%s/power1_input", base))
							}
							if len(pBytes) > 0 {
								if pVal, errParse := strconv.ParseFloat(strings.TrimSpace(string(pBytes)), 64); errParse == nil && pVal > 0 {
									watts := pVal / 1000000.0
									capBytes, errC := os.ReadFile(fmt.Sprintf("%s/power1_cap", base))
									if errC == nil {
										if capVal, errCap := strconv.ParseFloat(strings.TrimSpace(string(capBytes)), 64); errCap == nil && capVal > 0 {
											capWatts := capVal / 1000000.0
											if details.PowerState != "" {
												details.PowerState = fmt.Sprintf("%s (%.1f W / %.1f W)", details.PowerState, watts, capWatts)
											} else {
												details.PowerState = fmt.Sprintf("D0 (%.1f W / %.1f W)", watts, capWatts)
											}
										}
									}
									if !strings.Contains(details.PowerState, "W") {
										if details.PowerState != "" {
											details.PowerState = fmt.Sprintf("%s (%.1f W)", details.PowerState, watts)
										} else {
											details.PowerState = fmt.Sprintf("D0 (%.1f W)", watts)
										}
									}
									break
								}
							}
						}
					}
				}

				// Displays & Monitors
				if details.ActiveDisplays == "" {
					var displays []string
					for _, subF := range files {
						if strings.HasPrefix(subF.Name(), bestCard+"-") {
							connectorName := strings.TrimPrefix(subF.Name(), bestCard+"-")
							statusBytes, errSt := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/status", subF.Name()))
							if errSt == nil && strings.TrimSpace(string(statusBytes)) == "connected" {
								modesBytes, _ := os.ReadFile(fmt.Sprintf("/sys/class/drm/%s/modes", subF.Name()))
								mode := ""
								if len(modesBytes) > 0 {
									modeLines := strings.Split(strings.TrimSpace(string(modesBytes)), "\n")
									if len(modeLines) > 0 {
										mode = strings.TrimSpace(modeLines[0])
									}
								}
								if mode != "" {
									displays = append(displays, fmt.Sprintf("%s (%s, Connected)", connectorName, mode))
								} else {
									displays = append(displays, fmt.Sprintf("%s (Connected)", connectorName))
								}
							}
						}
					}
					if len(displays) > 0 {
						details.ActiveDisplays = strings.Join(displays, ", ")
					}
				}

				// VRAM (AMD / Sysfs)
				if details.VRAMTotal == "" {
					vramUsedPath := fmt.Sprintf("%s/mem_info_vram_used", devicePath)
					vramTotalPath := fmt.Sprintf("%s/mem_info_vram_total", devicePath)
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
								if details.VRAMType == "" {
									details.VRAMType = "Dedicated VRAM"
								}
							}
						}
					}

					// Intel Arc discrete VRAM (lmem)
					if details.VRAMTotal == "" {
						lmemBytes, errL := os.ReadFile(fmt.Sprintf("%s/lmem_total_bytes", devicePath))
						if errL == nil {
							if lVal, errParse := strconv.ParseUint(strings.TrimSpace(string(lmemBytes)), 10, 64); errParse == nil && lVal > 0 {
								details.VRAMTotal = FormatBytes(lVal)
								details.VRAMType = "Dedicated GDDR6"
							}
						}
					}
				}

				// Temperature
				if details.Temperature == "" {
					if tempStr := getAMDOrIntelTempFormatted(bestCard); tempStr != "" {
						details.Temperature = tempStr
					}
				}
			}
		}

		// Raspberry Pi / ARM SoC fallback
		if details.CurrentClock == "" || details.Temperature == "" {
			if out := runCommand("vcgencmd", "measure_temp"); strings.HasPrefix(out, "temp=") {
				details.Temperature = strings.TrimPrefix(out, "temp=")
			}
			if out := runCommand("vcgencmd", "measure_clock", "core"); strings.Contains(out, "=") {
				parts := strings.Split(out, "=")
				if len(parts) > 1 {
					if hz, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil && hz > 0 {
						details.CurrentClock = fmt.Sprintf("%.0f MHz", hz/1000000.0)
					}
				}
			}
			if details.VRAMTotal == "" {
				if out := runCommand("vcgencmd", "get_mem", "gpu"); strings.HasPrefix(out, "gpu=") {
					details.VRAMTotal = strings.TrimPrefix(out, "gpu=")
					details.VRAMType = "Shared System Memory"
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		// 1. Check if Nvidia GPU with nvidia-smi
		if nName, nVer, nBus, nTot, nUsed, nFree, nTemp, nPwr, nClk, nDisp, ok := getNvidiaDetails(); ok {
			if nName != "" {
				details.Name = nName
			}
			details.Vendor = "Nvidia"
			details.Driver = "Nvidia Proprietary"
			if nVer != "" {
				details.DriverVersion = nVer
			}
			if nBus != "" {
				details.PCIBus = nBus
			}
			if nTot != "" {
				details.VRAMTotal = nTot
				details.VRAMUsed = nUsed
				details.VRAMFree = nFree
			}
			if nTemp != "" {
				details.Temperature = nTemp
			}
			if nPwr != "" {
				details.PowerState = nPwr
			}
			if nClk != "" {
				details.CurrentClock = nClk
			}
			if nDisp != "" && details.ActiveDisplays == "" {
				details.ActiveDisplays = nDisp
			}
		}

		// 2. Query Windows WMI / CIM for GPU details (AMD, Intel, Nvidia, Qualcomm)
		getWindowsGPUDetails(details)

		// 3. DirectX
		details.DirectX = "DirectX 12"
	} else if runtime.GOOS == "darwin" {
		getDarwinGPUDetails(details)
	} else if runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
		getBSDGPUDetails(details)
	}

	if details.Vendor == "" {
		lower := strings.ToLower(details.Name)
		if strings.Contains(lower, "intel") {
			details.Vendor = "Intel"
		} else if strings.Contains(lower, "nvidia") || strings.Contains(lower, "geforce") {
			details.Vendor = "Nvidia"
		} else if strings.Contains(lower, "amd") || strings.Contains(lower, "radeon") || strings.Contains(lower, "ati") {
			details.Vendor = "AMD"
		} else if strings.Contains(lower, "apple") {
			details.Vendor = "Apple"
		} else if strings.Contains(lower, "broadcom") || strings.Contains(lower, "videocore") {
			details.Vendor = "Broadcom"
		} else if strings.Contains(lower, "qualcomm") || strings.Contains(lower, "adreno") {
			details.Vendor = "Qualcomm"
		} else if strings.Contains(lower, "arm") || strings.Contains(lower, "mali") {
			details.Vendor = "ARM"
		}
	}

	return details
}

func getOpenGLDetails() (version, renderer, coreProfile, esProfile, vram string, unified bool) {
	out := runCommand("glxinfo", "-B")
	if out == "" {
		out = runCommand("glxinfo")
	}
	if out != "" {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "OpenGL version string:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL version string:"))
			} else if strings.HasPrefix(line, "OpenGL core profile version string:") {
				coreProfile = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL core profile version string:"))
			} else if strings.HasPrefix(line, "OpenGL renderer string:") {
				renderer = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL renderer string:"))
			} else if strings.HasPrefix(line, "OpenGL ES profile version string:") {
				esProfile = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL ES profile version string:"))
			} else if strings.HasPrefix(line, "Video memory:") {
				vram = strings.TrimSpace(strings.TrimPrefix(line, "Video memory:"))
			} else if strings.HasPrefix(line, "Unified memory:") {
				if strings.Contains(line, "yes") {
					unified = true
				}
			}
		}
	}

	if version == "" {
		out = runCommand("eglinfo")
		if out != "" {
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "EGL version string:") {
					version = strings.TrimSpace(strings.TrimPrefix(line, "EGL version string:"))
					break
				}
			}
		}
	}
	return
}

func getVulkanDetails(vendorHint string) (version, driver string) {
	out := runCommand("vulkaninfo", "--summary")
	if out == "" {
		out = runCommand("vulkaninfo")
	}
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Vulkan Instance Version:") {
				version = strings.TrimSpace(strings.TrimPrefix(line, "Vulkan Instance Version:"))
			} else if strings.HasPrefix(line, "driverName") {
				parts := strings.Split(line, "=")
				if len(parts) > 1 {
					driver = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	// Zero-subprocess Pure Go Fallback: parse ICD JSON manifests
	if version == "" || driver == "" {
		vHint := strings.ToLower(vendorHint)
		icdDirs := []string{"/usr/share/vulkan/icd.d", "/etc/vulkan/icd.d"}
		var bestScore int = -100
		var bestVer, bestDrv string

		for _, dir := range icdDirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".json") {
					data, err := os.ReadFile(fmt.Sprintf("%s/%s", dir, entry.Name()))
					if err == nil {
						var icdManifest struct {
							ICD struct {
								APIVersion  string `json:"api_version"`
								LibraryPath string `json:"library_path"`
							} `json:"ICD"`
						}
						if err := json.Unmarshal(data, &icdManifest); err == nil && icdManifest.ICD.APIVersion != "" {
							nameL := strings.ToLower(entry.Name())
							score := 0
							if strings.Contains(nameL, "x86_64") || strings.Contains(nameL, "64") {
								score += 10
							}
							if strings.Contains(nameL, "i686") || strings.Contains(nameL, "32") {
								score -= 10
							}

							var drvName string
							if strings.Contains(nameL, "intel") {
								drvName = "Mesa Intel (ANV)"
								if strings.Contains(vHint, "intel") {
									score += 50
								}
							} else if strings.Contains(nameL, "radeon") {
								drvName = "Mesa AMD (RADV)"
								if strings.Contains(vHint, "amd") || strings.Contains(vHint, "radeon") {
									score += 50
								}
							} else if strings.Contains(nameL, "nvidia") {
								drvName = "NVIDIA Proprietary"
								if strings.Contains(vHint, "nvidia") {
									score += 50
								}
							} else if strings.Contains(nameL, "nouveau") {
								drvName = "Mesa Nouveau (NVK)"
								if strings.Contains(vHint, "nvidia") {
									score += 30
								}
							} else if strings.Contains(nameL, "lvp") {
								drvName = "Mesa Lavapipe (CPU)"
							} else if strings.Contains(nameL, "asahi") {
								drvName = "Mesa Asahi (Honeykrisp)"
								if strings.Contains(vHint, "apple") {
									score += 50
								}
							} else if strings.Contains(nameL, "broadcom") {
								drvName = "Mesa Broadcom (V3DV)"
								if strings.Contains(vHint, "broadcom") {
									score += 50
								}
							} else if strings.Contains(nameL, "panfrost") {
								drvName = "Mesa Panfrost (PanVK)"
								if strings.Contains(vHint, "arm") || strings.Contains(vHint, "mali") {
									score += 50
								}
							} else {
								drvName = strings.TrimSuffix(entry.Name(), ".json")
							}

							if score > bestScore {
								bestScore = score
								bestVer = icdManifest.ICD.APIVersion
								bestDrv = drvName
							}
						}
					}
				}
			}
		}

		if version == "" {
			version = bestVer
		}
		if driver == "" {
			driver = bestDrv
		}
	}

	return
}

func getOpenCLDetails() string {
	out := runCommand("clinfo", "-l")
	if out == "" {
		out = runCommand("clinfo")
	}
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Platform Version") || strings.Contains(line, "OpenCL ") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}

	// Check ICD manifests in /etc/OpenCL/vendors
	icdDirs := []string{"/etc/OpenCL/vendors", "/usr/share/OpenCL/vendors"}
	var foundVendors []string
	for _, dir := range icdDirs {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".icd") {
					v := strings.TrimSuffix(e.Name(), ".icd")
					foundVendors = append(foundVendors, v)
				}
			}
		}
	}
	if len(foundVendors) > 0 {
		return fmt.Sprintf("Available (%s)", strings.Join(foundVendors, ", "))
	}

	return ""
}

func getCUDAVersion() string {
	out := runCommand("nvcc", "--version")
	if out != "" {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "release") {
				idx := strings.Index(line, "release")
				return strings.TrimSpace(line[idx:])
			}
		}
	}
	out = runCommand("nvidia-smi")
	if out != "" {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "CUDA Version:") {
				idx := strings.Index(line, "CUDA Version:")
				part := line[idx:]
				part = strings.TrimSuffix(part, "|")
				return strings.TrimSpace(strings.TrimPrefix(part, "CUDA Version:"))
			}
		}
	}
	return ""
}

