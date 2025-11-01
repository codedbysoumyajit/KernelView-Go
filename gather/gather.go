package gather

import (
	"encoding/json" // For parsing ip-api response
	"fmt"
	"net"
	"net/http" // For ip-api request
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ping/ping" // Import ping package
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process" // Import process package
)

// SystemInfo holds all collected system data. Exported for use in main.
type SystemInfo struct {
	OS             string
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
	Temperature    string // New: Slow
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
	Ping       string // e.g., "1.1.1.1: 15.2 ms"
	IOCounters string // New: Network I/O
}

// Struct to parse ip-api.com response
type ipAPIResponse struct {
	Status      string  `json:"status"`
	Country     string  `json:"country"`
	City        string  `json:"city"`
	ISP         string  `json:"isp"`
	Query       string  `json:"query"` // This holds the public IP
	Message     string  `json:"message"` // For error reporting
}


// --- Internal Helper Functions ---

func runCommand(name string, arg ...string) string {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = nil // Suppress errors
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
	cmd.Stderr = nil // Suppress errors
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Helper to format bytes into readable string (GB, MB, KB)
func formatBytes(b uint64) string {
    if b > (1 << 30) { // Greater than 1 GB
        return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
    }
    if b > (1 << 20) { // Greater than 1 MB
        return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
    }
    if b > (1 << 10) { // Greater than 1 KB
        return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
    }
    return fmt.Sprintf("%d B", b)
}


// --- Gathering Functions ---

// Simplified CPU Info - Relies solely on gopsutil
func getCPUInfoDetailed() string {
	// gopsutil cpu.Info() is generally the most straightforward cross-platform method
	if c, err := cpu.Info(); err == nil && len(c) > 0 {
		return c[0].ModelName
	}
	return "Unknown Processor"
}

func gatherHostInfo(info *SystemInfo, wg *sync.WaitGroup) {
	defer wg.Done()
	h, err := host.Info()
	if err != nil {
		return
	}
	uptimeDuration := time.Second * time.Duration(h.Uptime)
	days := int(uptimeDuration.Hours() / 24)
	hours := int(uptimeDuration.Hours()) % 24
	minutes := int(uptimeDuration.Minutes()) % 60
	if days > 0 {
		info.Uptime = fmt.Sprintf("%d days, %d hours", days, hours)
	} else if hours > 0 {
		info.Uptime = fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	} else {
		info.Uptime = fmt.Sprintf("%d minutes", minutes)
	}
	info.OS = getOSInfo() // OS info fetched once here
	kernelName := h.Platform
	if kernelName == "windows" {
		kernelName = "Windows NT"
	}
	info.Kernel = fmt.Sprintf("%s %s", strings.Title(kernelName), h.KernelVersion)
	info.Hostname, _ = os.Hostname()
}

func gatherCPUInfo(info *SystemInfo, wg *sync.WaitGroup, isFast bool) {
	defer wg.Done()
	info.CPU = getCPUInfoDetailed() // Calls the simplified version now
	if cpuStats, err := cpu.Info(); err == nil && len(cpuStats) > 0 {
		mhz := cpuStats[0].Mhz
		if mhz > 1000 {
			info.CPUSpeed = fmt.Sprintf("%.2f GHz", mhz/1000.0)
		} else {
			info.CPUSpeed = fmt.Sprintf("%.0f MHz", mhz)
		}
	}
	cores, _ := cpu.Counts(false) // Physical cores
	threads, _ := cpu.Counts(true) // Logical cores (threads)
	info.CoresThreads = fmt.Sprintf("%d/%d", cores, threads)

	if !isFast {
		percentages, err := cpu.Percent(150*time.Millisecond, false)
		if err == nil && len(percentages) > 0 {
			info.CPUUsage = fmt.Sprintf("%.1f%%", percentages[0])
		} else {
			info.CPUUsage = "N/A"
		}
	}
}

func gatherMemoryInfo(info *SystemInfo, wg *sync.WaitGroup) {
	defer wg.Done()
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

func getOSInfo() string {
	switch runtime.GOOS {
	case "linux":
		if content, err := os.ReadFile("/etc/os-release"); err == nil {
			re := regexp.MustCompile(`PRETTY_NAME="([^"]+)"`)
			if match := re.FindStringSubmatch(string(content)); len(match) > 1 {
				return match[1]
			}
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

func getGPUInfo() string {
	switch runtime.GOOS {
	case "windows":
		return runShellCommand("(Get-CimInstance Win32_VideoController).Caption")
	case "linux":
		output := runShellCommand("lspci -mm | grep -i 'VGA\\|3D\\|Display' | head -n1 | cut -d '\"' -f2,4 | sed 's/\" \"/ /'")
		if output != "" {
			return output
		}
		output = runShellCommand("lspci | grep -i 'VGA\\|3D\\|Display' | head -n1 | cut -d ':' -f3 | sed 's/ (rev ..)//;s/\\[.*\\]//'")
		return strings.TrimSpace(output)
	case "darwin":
		output := runShellCommand("system_profiler SPDisplaysDataType | grep 'Chipset Model' | cut -d ':' -f2")
		return strings.TrimSpace(output)
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
		if conn.Status == "LISTEN" && conn.Laddr.IP != "::" && conn.Laddr.IP != "0.0.0.0" {
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
	limit := 5
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
	mu := &sync.Mutex{}
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

func getResolution() string {
	switch runtime.GOOS {
	case "windows":
		output := runShellCommand("(Get-CimInstance Win32_VideoController).CurrentHorizontalResolution,(Get-CimInstance Win32_VideoController).CurrentVerticalResolution -join 'x'")
		if output != "" {
			return output
		}
	case "linux":
		if os.Getenv("DISPLAY") != "" {
			output := runShellCommand("xrandr --current | grep '*' | uniq | awk '{print $1}'")
			if output != "" {
				return output
			}
		}
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			return "Wayland (res?)"
		}
		return "Headless"
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

func getWindowManager() string {
	if runtime.GOOS == "linux" {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			session := os.Getenv("XDG_SESSION_TYPE")
			if session == "wayland" {
				currentDesktop := os.Getenv("XDG_CURRENT_DESKTOP")
				switch strings.ToLower(currentDesktop) {
				case "gnome": return "Mutter (Wayland)"
				case "kde": return "KWin (Wayland)"
				case "sway": return "Sway"
				case "wlroots": return "wlroots based"
				}
				return "Wayland"
			}
		}
		desktopSession := os.Getenv("DESKTOP_SESSION")
		if desktopSession != "" {
			lowerSession := strings.ToLower(desktopSession)
			if strings.Contains(lowerSession, "gnome") { return "Mutter (X11)" }
			if strings.Contains(lowerSession, "kde") || strings.Contains(lowerSession, "plasma") { return "KWin (X11)" }
			if strings.Contains(lowerSession, "xfce") { return "Xfwm4" }
			if strings.Contains(lowerSession, "cinnamon") { return "Muffin" }
			if strings.Contains(lowerSession, "mate") { return "Marco" }
			if strings.Contains(lowerSession, "lxqt") { return "Openbox" }
			return strings.Title(desktopSession)
		}
		if wm := runShellCommand("wmctrl -m | grep 'Name:'"); wm != "" {
			return strings.TrimSpace(strings.Split(wm, ":")[1])
		}
		return "Unknown (X11?)"
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
	var checkers map[string]string
	switch runtime.GOOS {
	case "linux":
		checkers = map[string]string{
			"APT": "dpkg-query -f . -W | wc -l", "Pacman": "pacman -Qq --color never | wc -l",
			"DNF": "dnf list installed --quiet | wc -l", "Flatpak": "flatpak list --app --columns=application | wc -l",
			"Snap": "snap list | tail -n +2 | wc -l",
		}
	case "darwin":
		checkers = map[string]string{
			"Brew": "brew list --formula | wc -l",
			"Cask": "brew list --cask | wc -l",
		}
	case "windows":
		checkers = map[string]string{
			"Choco": "(choco list -l | Measure-Object).Count", "Winget": "(winget list | Measure-Object).Count",
			"Scoop": "(scoop list | Measure-Object).Count",
		}
	default:
		return "None detected"
	}
	var wg sync.WaitGroup
	results := make(chan string, len(checkers))
	for name, cmd := range checkers {
		wg.Add(1)
		go func(n, c string) {
			defer wg.Done()
			baseCmd := strings.Fields(strings.Split(c, "|")[0])[0]
			if _, err := exec.LookPath(baseCmd); err != nil && baseCmd != "(" {
				return
			}
			countStr := runShellCommand(c)
			if countStr != "" {
				countStr = strings.TrimSpace(countStr)
				if count, err := strconv.Atoi(countStr); err == nil && count > 0 {
					results <- fmt.Sprintf("%s (%d)", n, count)
				}
			}
		}(name, cmd)
	}
	wg.Wait()
	close(results)
	var parts []string
	for res := range results {
		parts = append(parts, res)
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return "None detected"
	}
	return strings.Join(parts, ", ")
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
		Timeout: 800 * time.Millisecond,
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

func getPingTime(host string) string {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		return fmt.Sprintf("%s: Error", host)
	}

	pinger.Count = 1
	pinger.Timeout = 800 * time.Millisecond
	pinger.SetPrivileged(runtime.GOOS != "windows")

	err = pinger.Run()
	if err != nil {
        if strings.Contains(err.Error(), "operation not permitted") || strings.Contains(err.Error(), "requires administrator privileges") {
             return fmt.Sprintf("%s: Need Privileges", host)
        }
		return fmt.Sprintf("%s: Failed", host)
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv > 0 {
		return fmt.Sprintf("%s: %.1f ms", host, float64(stats.AvgRtt.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%s: Timeout", host)
}

// ** NEW FUNCTION **
func getIOCounters() string {
	// true = per interface
	counters, err := psnet.IOCounters(true)
	if err != nil {
		return "N/A"
	}
	
	// Get list of interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "N/A"
	}
	
	// Find the primary, active, non-loopback interface
	for _, iface := range ifaces {
		// Skip down, loopback, and virtual interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.Contains(strings.ToLower(iface.Name), "virtual") || strings.Contains(strings.ToLower(iface.Name), "docker") || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		
		// Find the counters for this interface
		for _, counter := range counters {
			if counter.Name == iface.Name {
				// Found it. Format and return.
				return fmt.Sprintf("%s (Sent: %s, Recv: %s)", 
					iface.Name, 
					formatBytes(counter.BytesSent), 
					formatBytes(counter.BytesRecv))
			}
		}
	}
	
	// Fallback to all (sum of) interfaces if no clear primary found
	if len(counters) > 0 {
		return fmt.Sprintf("All (Sent: %s, Recv: %s)",
			formatBytes(counters[0].BytesSent), 
			formatBytes(counters[0].BytesRecv))
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
		"Shell": getShell, "GPU": getGPUInfo, "Disk": getDisk, "IPAddress": getIPAddress,
		"Locale": getSystemLocale, "Resolution": getResolution, "WindowManager": getWindowManager,
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
	pids, err := process.Pids()
	if err != nil {
		return nil, fmt.Errorf("failed to get PIDs: %w", err)
	}

	var processList []ProcessInfo
	var wg sync.WaitGroup
	results := make(chan ProcessInfo, len(pids))

	// Prime CPU Percent calculation
	allProcs, _ := process.Processes()
	for _, p := range allProcs {
		_, _ = p.CPUPercent() // Initial call to start measurement
	}
	time.Sleep(150 * time.Millisecond) // Adjusted sleep duration


	for _, pid := range pids {
		wg.Add(1)
		go func(p int32) {
			defer wg.Done()
			proc, err := process.NewProcess(p)
			if err != nil {
				return
			}

			name, _ := proc.Name() 
			if name == "" || p <= 1 || (runtime.GOOS == "linux" && strings.HasPrefix(name, "[") ) || name == "kernel_task" || name == "systemd" || name == "init" || name == "idle" {
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
					PID:     p,
					Name:    name,
					CPU:     cpuPerc,
					RAM:     memInfo.RSS,
					RAMPerc: memPerc,
				}
			}
		}(pid)
	}

	wg.Wait()
	close(results)

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
		info.Ping = getPingTime("1.1.1.1") // Ping Cloudflare
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
		info.IOCounters = getIOCounters() // ** ADDED **
	}()


	wg.Wait()

	if errPublicIP != nil {
		info.PublicIP = "Error"
		info.ISP = "Error"
		info.City = "Error"
		info.Country = "Error"
		// This now runs in the background, so we just log the error
		// fmt.Fprintf(os.Stderr, "Warning: Could not fetch public IP info: %v\n", errPublicIP)
	}

	return info, nil
}
