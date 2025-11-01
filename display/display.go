package display

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/term" // Import the terminal package
	"KernelView-Go/gather"
)

// Theme struct to hold color definitions (exported)
type Theme struct {
	Category string
	Key      string
	Value    string
	Accent   string
	Reset    string
}

// Define the two themes (exported)
var (
	NormalTheme = Theme{
		Category: "\033[34m",      // Bright Blue
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[34m",      // Bright Blue
		Reset:    "\033[0m",
	}
	FastTheme = Theme{
		Category: "\033[36m",      // Bright Cyan
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[36m",      // Bright Cyan
		Reset:    "\033[0m",
	}
	// New: BlankTheme for --no-color
	BlankTheme = Theme{
		Category: "",
		Key:      "",
		Value:    "",
		Accent:   "",
		Reset:    "",
	}
)

// --- Internal Helper Functions ---

func stripAnsi(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func Max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		if maxLen < 1 {
			maxLen = 1
		}
		return string(runes[:maxLen-1]) + "…" // Use ellipsis character
	}
	return s
}


// Get terminal width helper
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // Default width
	}
	return width
}


// --- Display Functions ---

// DisplaySystemInfo formats and prints the info (exported).
func DisplaySystemInfo(info *gather.SystemInfo, theme Theme) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J\033[3J") // Clear screen
	}

	type infoEntry struct{ Key, Value string }
	groups := []struct {
		Category string
		Items    []infoEntry
	}{
		{"System", []infoEntry{{"OS", info.OS}, {"Kernel", info.Kernel}, {"Virtualization", info.Virtualization}, {"Uptime", info.Uptime}, {"Shell", info.Shell}, {"Terminal", info.Terminal}}},
		{"Hardware", []infoEntry{{"CPU", info.CPU}, {"GPU", info.GPU}, {"RAM", info.RAM}}},
		{"Network", []infoEntry{{"Hostname", info.Hostname}, {"IP Address", info.IPAddress}}},
		{"Storage", []infoEntry{{"Disk", info.Disk}, {"Swap", info.Swap}}},
		{"Display", []infoEntry{{"Resolution", info.Resolution}, {"DE", info.DE}, {"WM", info.WindowManager}}},
		{"Software", []infoEntry{{"Packages", info.Packages}, {"Languages", info.Languages}, {"Go", info.Go}}},
		{"CPU Stats", []infoEntry{{"Cores/Threads", info.CoresThreads}, {"Speed", info.CPUSpeed}, {"Usage", info.CPUUsage}, {"Temperature", info.Temperature}}},
		{"Other", []infoEntry{{"Locale", info.Locale}, {"Ports", info.OpenPorts}}},
	}

	var formattedLines []string
	maxKeyLen := 0
	// Filter and prepare lines first
	for i := range groups {
		var groupLines []string
		groupHasContent := false
		for _, item := range groups[i].Items {
			if item.Value != "" && item.Value != "Unknown" && item.Value != "None" && item.Value != "N/A" && item.Value != "0GB/0GB (0.0%)" && item.Value != "0GB / 0GB (0.0%)" && item.Value != "None detected" {
				if !groupHasContent {
					groupLines = append(groupLines, fmt.Sprintf("%s─── %s ───%s", theme.Category, groups[i].Category, theme.Reset))
					groupHasContent = true
				}
				if len(item.Key) > maxKeyLen {
					maxKeyLen = len(item.Key)
				}
				groupLines = append(groupLines, fmt.Sprintf("%s:%s", item.Key, item.Value))
			}
		}
		formattedLines = append(formattedLines, groupLines...)
	}

	finalFormattedLines := []string{}
	maxInfoWidth := 0
	for _, line := range formattedLines {
		if strings.Contains(line, "───") { // Header line
			finalFormattedLines = append(finalFormattedLines, line)
			if len(stripAnsi(line)) > maxInfoWidth {
				maxInfoWidth = len(stripAnsi(line))
			}
		} else if strings.Contains(line, ":") { // Key-value line
			parts := strings.SplitN(line, ":", 2)
			key := parts[0]
			value := parts[1]
			padding := strings.Repeat(" ", maxKeyLen-len(key))
			formattedLine := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, key, padding, theme.Value, value, theme.Reset)
			finalFormattedLines = append(finalFormattedLines, formattedLine)
			if len(stripAnsi(formattedLine)) > maxInfoWidth {
				maxInfoWidth = len(stripAnsi(formattedLine))
			}
		}
	}

	// Print Title centered above the info block using terminal width
	title := "KernelView Go"
	termWidth := getTerminalWidth()
	titleSpacing := Max(0, (termWidth/2)-(len(title)/2)) // Center based on terminal width
	fmt.Printf("\n%s%s%s%s\n\n", strings.Repeat(" ", titleSpacing), theme.Accent, title, theme.Reset)

	// Print the formatted lines
	for _, line := range finalFormattedLines {
		fmt.Println(line)
	}
	fmt.Println() // Add a blank line at the bottom
}


// DisplayProcessList formats and prints the process list (Wider & Centered Title).
func DisplayProcessList(processList []gather.ProcessInfo, theme Theme) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J\033[3J") // Clear screen
	}

	termWidth := getTerminalWidth()

	// --- Add Centered Title ---
	title := "KernelView Go - Processes"
	titleSpacing := Max(0, (termWidth/2)-(len(title)/2))
	fmt.Printf("\n%s%s%s%s\n\n", strings.Repeat(" ", titleSpacing), theme.Accent, title, theme.Reset)
	// ---------------

	// --- Adjust Column Widths ---
	pidWidth := 8       // Fixed width for PID
	cpuWidth := 8       // Fixed width for CPU%
	ramWidth := 8       // Fixed width for RAM
	nameWidth := termWidth - pidWidth - cpuWidth - ramWidth - 5 // Dynamic name width
	minNameWidth := 20
	if nameWidth < minNameWidth {
		nameWidth = minNameWidth
	}
	totalWidth := pidWidth + nameWidth + cpuWidth + ramWidth + 3
	if totalWidth > termWidth {
		nameWidth -= (totalWidth - termWidth) // Shrink name if total is still too wide
	}
	if nameWidth < minNameWidth { nameWidth = minNameWidth }
	totalWidth = pidWidth + nameWidth + cpuWidth + ramWidth + 3


	// Print Header (Wider)
	fmt.Printf("%s%-*s %-*s %*s %*s%s\n",
		theme.Key, pidWidth, "PID", nameWidth, "NAME", cpuWidth, "CPU%", ramWidth, "RAM", theme.Reset)
	fmt.Printf("%s%s%s\n", theme.Category, strings.Repeat("─", totalWidth), theme.Reset) // Wider Separator

	limit := 30
	if len(processList) < limit {
		limit = len(processList)
	}

	for _, p := range processList[:limit] {
		// Format RAM
		var ramStr string
		ramMB := float64(p.RAM) / (1024 * 1024)
		if ramMB < 1.0 {
			ramKB := float64(p.RAM) / 1024
			ramStr = fmt.Sprintf("%.0fK", ramKB)
		} else if ramMB < 1024 {
			ramStr = fmt.Sprintf("%.0fM", ramMB)
		} else {
			ramGB := ramMB / 1024
			ramStr = fmt.Sprintf("%.1fG", ramGB)
		}

		// Print process row
		fmt.Printf("%s%-*d %s%-*s %s%*.1f%% %s%*s %s%s\n",
			theme.Value, pidWidth, p.PID,
			theme.Key, nameWidth, truncateString(p.Name, nameWidth),
			theme.Value, cpuWidth-1, p.CPU,
			theme.Value, ramWidth, ramStr,
			theme.Reset, "")
	}
	fmt.Println()
}

// DisplayNetworkInfo formats and prints detailed network info (Exported)
func DisplayNetworkInfo(info *gather.NetworkInfo, theme Theme) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J\033[3J") // Clear screen
	}

	termWidth := getTerminalWidth()

	// --- Add Centered Title ---
	title := "KernelView Go - Network Details"
	titleSpacing := Max(0, (termWidth/2)-(len(title)/2))
	fmt.Printf("\n%s%s%s%s\n\n", strings.Repeat(" ", titleSpacing), theme.Accent, title, theme.Reset)
	// ---------------

	// Prepare data - find max key length
	items := []struct{ Key, Value string }{
		{"Hostname", info.Hostname},
		{"Private IP", info.PrivateIP},
		{"MAC Address", info.MACAddress},
		{"I/O Counters", info.IOCounters}, // ** ADDED **
		{"Public IP", info.PublicIP},
		{"ISP", info.ISP},
		{"Location", fmt.Sprintf("%s, %s", info.City, info.Country)},
		{"Proxy", info.Proxy},
		{"DNS Servers", strings.Join(info.DNSServers, ", ")},
		{"Ping (1.1.1.1)", info.Ping},
	}

	maxKeyLen := 0
	validItems := []struct{ Key, Value string }{} // Filter out empty values
	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "N/A" && val != "Error" && val != "," && val != "Error, Error" {
			if item.Key == "Location" && (info.City == "" || info.City == "Error") && (info.Country == "" || info.Country == "Error") {
				continue
			}
			if item.Key == "Location" && (info.City == "" || info.City == "Error") {
				item.Value = info.Country
			}
			if item.Key == "Location" && (info.Country == "" || info.Country == "Error") {
				item.Value = info.City
			}

			validItems = append(validItems, item)
			if len(item.Key) > maxKeyLen {
				maxKeyLen = len(item.Key)
			}
		}
	}

	// Print lines
	for _, item := range validItems {
		padding := strings.Repeat(" ", maxKeyLen-len(item.Key))
		fmt.Printf("%s%s%s: %s%s%s\n",
			theme.Key, item.Key, padding,
			theme.Value, item.Value, theme.Reset)
	}

	fmt.Println() // Add a blank line at the bottom
}
