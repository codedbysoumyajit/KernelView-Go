package display

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"KernelView-Go/gather" // Import the gather package to use SystemInfo
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
		Category: "\033[34m",
		Key:      "\033[38;5;255m",
		Value:    "\033[38;5;249m",
		Accent:   "\033[34m",
		Reset:    "\033[0m",
	}
	FastTheme = Theme{
		Category: "\033[36m",
		Key:      "\033[38;5;255m",
		Value:    "\033[38;5;249m",
		Accent:   "\033[36m",
		Reset:    "\033[0m",
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

// --- Display Function ---

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
		// {"Network", []infoEntry{{"Hostname", info.Hostname}, {"IP Address", info.IPAddress}, {"Speed", info.NetworkSpeed}}}, // Speed REMOVED
		{"Network", []infoEntry{{"Hostname", info.Hostname}, {"IP Address", info.IPAddress}}}, // Corrected Network group
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

	// Print Title centered above the info block
	title := "KernelView Go"
	if maxInfoWidth > 0 {
		titleSpacing := Max(0, (maxInfoWidth/2)-(len(title)/2))
		fmt.Printf("\n%s%s%s%s\n\n", strings.Repeat(" ", titleSpacing), theme.Accent, title, theme.Reset)
	}

	// Print the formatted lines
	for _, line := range finalFormattedLines {
		fmt.Println(line)
	}
	fmt.Println() // Add a blank line at the bottom
}
