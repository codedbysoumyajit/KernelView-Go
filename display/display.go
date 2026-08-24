package display

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"KernelView-Go/gather"
	"golang.org/x/term"
)

// Theme struct to hold color definitions (exported)
type Theme struct {
	Category string
	Key      string
	Value    string
	Accent   string
	Reset    string
}

// Define the standard themes (exported)
var (
	NormalTheme = Theme{
		Category: "\033[34m",       // Bright Blue
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[34m",       // Bright Blue
		Reset:    "\033[0m",
	}
	FastTheme = Theme{
		Category: "\033[36m",       // Bright Cyan
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[36m",       // Bright Cyan
		Reset:    "\033[0m",
	}
	BlankTheme = Theme{
		Category: "",
		Key:      "",
		Value:    "",
		Accent:   "",
		Reset:    "",
	}
)

var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

// --- Internal Helper Functions ---

func stripAnsi(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
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

func getUsername() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "user"
}

func getDistroKey() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	if runtime.GOOS == "freebsd" {
		return "freebsd"
	}
	if runtime.GOOS == "openbsd" {
		return "openbsd"
	}
	if runtime.GOOS == "netbsd" {
		return "netbsd"
	}

	// Read standard /etc/os-release or /usr/lib/os-release
	osReleasePaths := []string{"/etc/os-release", "/usr/lib/os-release"}
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		osReleasePaths = append(osReleasePaths, prefix+"/etc/os-release")
	}

	for _, path := range osReleasePaths {
		if content, err := os.ReadFile(path); err == nil {
			contentStr := strings.ToLower(string(content))
			lines := strings.Split(contentStr, "\n")
			var id, idLike string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "id=") {
					id = strings.Trim(strings.TrimPrefix(line, "id="), "\"'\r ")
				}
				if strings.HasPrefix(line, "id_like=") {
					idLike = strings.Trim(strings.TrimPrefix(line, "id_like="), "\"'\r ")
				}
			}

			// Exact ID match first
			switch id {
			case "manjaro":
				return "manjaro"
			case "arch":
				return "arch"
			case "pop":
				return "pop"
			case "nixos":
				return "nixos"
			case "void":
				return "void"
			case "ubuntu":
				return "ubuntu"
			case "debian":
				return "debian"
			case "fedora":
				return "fedora"
			case "centos":
				return "centos"
			case "rhel", "redhat":
				return "redhat"
			case "alpine":
				return "alpine"
			case "gentoo":
				return "gentoo"
			case "opensuse", "suse", "opensuse-tumbleweed", "opensuse-leap":
				return "opensuse"
			case "linuxmint", "mint":
				return "mint"
			}

			// Substring / id_like fallback
			combined := id + " " + idLike + " " + contentStr
			if strings.Contains(combined, "manjaro") {
				return "manjaro"
			}
			if strings.Contains(combined, "pop!_os") || strings.Contains(combined, "pop-os") || strings.Contains(combined, "pop_os") {
				return "pop"
			}
			if strings.Contains(combined, "nixos") {
				return "nixos"
			}
			if strings.Contains(combined, "void") {
				return "void"
			}
			if strings.Contains(combined, "arch") || strings.Contains(combined, "endeavouros") {
				return "arch"
			}
			if strings.Contains(combined, "ubuntu") {
				return "ubuntu"
			}
			if strings.Contains(combined, "debian") {
				return "debian"
			}
			if strings.Contains(combined, "fedora") {
				return "fedora"
			}
			if strings.Contains(combined, "centos") {
				return "centos"
			}
			if strings.Contains(combined, "redhat") || strings.Contains(combined, "red hat") || strings.Contains(combined, "rhel") {
				return "redhat"
			}
			if strings.Contains(combined, "alpine") {
				return "alpine"
			}
			if strings.Contains(combined, "gentoo") {
				return "gentoo"
			}
			if strings.Contains(combined, "suse") || strings.Contains(combined, "opensuse") {
				return "opensuse"
			}
			if strings.Contains(combined, "mint") {
				return "mint"
			}
		}
	}
	return "linux"
}

func GetThemeForDistro(distroKey string, noColor bool) Theme {
	if noColor {
		return BlankTheme
	}

	valColor := "\033[38;5;253m" // Light gray for values
	reset := "\033[0m"

	switch distroKey {
	case "manjaro":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "arch":
		return Theme{
			Category: "\033[1;36m", // Bold Cyan
			Key:      "\033[1;36m",
			Value:    valColor,
			Accent:   "\033[1;36m",
			Reset:    reset,
		}
	case "ubuntu":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "pop":
		return Theme{
			Category: "\033[1;36m", // Bold Cyan / Teal
			Key:      "\033[1;36m",
			Value:    valColor,
			Accent:   "\033[1;36m",
			Reset:    reset,
		}
	case "nixos":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold Soft Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "void":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "debian":
		return Theme{
			Category: "\033[1;38;5;161m", // Bold Crimson
			Key:      "\033[1;38;5;161m",
			Value:    valColor,
			Accent:   "\033[1;38;5;161m",
			Reset:    reset,
		}
	case "fedora":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold Soft Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "macos":
		return Theme{
			Category: "\033[1;35m", // Bold Purple
			Key:      "\033[1;35m",
			Value:    valColor,
			Accent:   "\033[1;35m",
			Reset:    reset,
		}
	case "windows":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Sky Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "freebsd":
		return Theme{
			Category: "\033[1;31m", // Bold Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "openbsd":
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	case "netbsd":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "alpine":
		return Theme{
			Category: "\033[1;34m", // Bold Blue
			Key:      "\033[1;34m",
			Value:    valColor,
			Accent:   "\033[1;34m",
			Reset:    reset,
		}
	case "gentoo":
		return Theme{
			Category: "\033[1;35m", // Bold Purple/Magenta
			Key:      "\033[1;35m",
			Value:    valColor,
			Accent:   "\033[1;35m",
			Reset:    reset,
		}
	case "redhat":
		return Theme{
			Category: "\033[1;31m", // Bold Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "opensuse":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "mint":
		return Theme{
			Category: "\033[1;38;5;120m", // Bold Mint Green
			Key:      "\033[1;38;5;120m",
			Value:    valColor,
			Accent:   "\033[1;38;5;120m",
			Reset:    reset,
		}
	case "centos":
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	default: // linux / generic
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	}
}

func getColorBlocks(theme Theme) string {
	if theme.Reset == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("  ")
	for i := 0; i < 8; i++ {
		sb.WriteString(fmt.Sprintf("\033[3%dm███", i))
	}
	sb.WriteString("\033[0m\n  ")
	for i := 0; i < 8; i++ {
		sb.WriteString(fmt.Sprintf("\033[9%dm███", i))
	}
	sb.WriteString("\033[0m")
	return sb.String()
}

// DisplayLayout prints the logo and info side-by-side.
func DisplayLayout(logoKey string, infoLines []string, theme Theme) {
	logoLines, exists := DistroLogos[logoKey]
	if !exists {
		logoLines = DistroLogos["linux"]
	}

	coloredLogoLines := make([]string, len(logoLines))
	for i, line := range logoLines {
		if theme.Accent != "" {
			coloredLogoLines[i] = theme.Accent + line + theme.Reset
		} else {
			coloredLogoLines[i] = line
		}
	}

	maxLines := len(coloredLogoLines)
	if len(infoLines) > maxLines {
		maxLines = len(infoLines)
	}

	logoWidth := 0
	for _, line := range logoLines {
		runesCount := utf8.RuneCountInString(line)
		if runesCount > logoWidth {
			logoWidth = runesCount
		}
	}

	termWidth := getTerminalWidth()

	// If the terminal is too narrow, fall back to pure text rendering without logo
	if termWidth < logoWidth+25 {
		for _, line := range infoLines {
			fmt.Println(line)
		}
		return
	}

	for i := 0; i < maxLines; i++ {
		var logoPart string
		if i < len(coloredLogoLines) {
			logoPart = coloredLogoLines[i]
		} else {
			logoPart = strings.Repeat(" ", logoWidth)
		}

		var rawLogoLen int
		if i < len(logoLines) {
			rawLogoLen = utf8.RuneCountInString(logoLines[i])
		} else {
			rawLogoLen = logoWidth
		}

		spacing := strings.Repeat(" ", logoWidth-rawLogoLen+3)

		var infoPart string
		if i < len(infoLines) {
			infoPart = infoLines[i]
		} else {
			infoPart = ""
		}

		// Ensure it doesn't overflow the screen width
		if logoWidth+3+utf8.RuneCountInString(stripAnsi(infoPart)) > termWidth {
			allowedWidth := termWidth - logoWidth - 4
			if allowedWidth > 10 {
				// Strip ANSI first to avoid broken escapes, truncate, and re-wrap
				clean := stripAnsi(infoPart)
				infoPart = theme.Value + truncateString(clean, allowedWidth) + theme.Reset
			}
		}

		fmt.Printf("%s%s%s\n", logoPart, spacing, infoPart)
	}
	fmt.Println() // Extra line break for bit space at the end of CLI
}

// --- Display Functions ---

// DisplaySystemInfo formats and prints the system info (exported).
func DisplaySystemInfo(info *gather.SystemInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	var infoLines []string

	username := getUsername()
	hostname := info.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)) + theme.Reset
	infoLines = append(infoLines, separator)

	items := []struct{ Key, Value string }{
		{"OS", info.OS},
		{"Host", info.Host},
		{"Kernel", info.Kernel},
		{"Uptime", info.Uptime},
		{"Packages", info.Packages},
		{"Shell", info.Shell},
		{"Resolution", info.Resolution},
		{"DE", info.DE},
		{"WM", info.WindowManager},
		{"Terminal", info.Terminal},
		{"CPU", info.CPU},
		{"GPU", info.GPU},
		{"Memory", info.RAM},
		{"Swap", info.Swap},
		{"Disk", info.Disk},
		{"Local IP", info.IPAddress},
		{"Locale", info.Locale},
		{"Languages", info.Languages},
		{"Open Ports", info.OpenPorts},
		{"Temperature", info.Temperature},
		{"Go Version", info.Go},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "Unknown" && val != "None" && val != "N/A" && val != "None detected" && val != "0GB/0GB (0.0%)" && val != "0GB / 0GB (0.0%)" {
			// Pre-truncate the value so it fits perfectly on the right panel
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}
			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// GPULogos holds vendor-specific ASCII art logos for graphics cards
var GPULogos = map[string][]string{
	"nvidia": {
		"  _   ___     _____ ____ ___    _   ",
		" | \\ | \\ \\   / /_ _|  _ \\_ _|  / \\  ",
		" |  \\| |\\ \\ / / | || | | | |  / _ \\ ",
		" | |\\  | \\ V /  | || |_| | | / ___ \\",
		" |_| \\_|  \\_/  |___|____/___/_/   \\_\\",
	},
	"amd": {
		"  _    __  __ ____  ",
		" / \\  |  \\/  |  _ \\ ",
		"/ _ \\ | |\\/| | | | |",
		" ___ \\| |  | | |_| |",
		"_/   \\_\\_|  |_|____/ ",
	},
	"intel": {
		" ___ _   _ _____ _____ _     ",
		"|_ _| \\ | |_   _| ____| |    ",
		" | ||  \\| | | | |  _| | |    ",
		" | || |\\  | | | | |___| |___ ",
		"|___|_| \\_| |_| |_____|_____|",
	},
	"qualcomm": {
		"   _  _ ___  ___ ___ _  _  ___  ",
		"  /_\\|   \\ _ \\| __| \\| |/ _ \\ ",
		" / _ \\ |) |   /| _|| .` | (_) |",
		"/_/ \\_\\___/|_|_\\___|_|\\_|\\___/ ",
	},
	"arm": {
		" __  __   _   _    ___ ",
		"|  \\/  | /_\\ | |  |_ _|",
		"| |\\/| |/ _ \\| |__ | | ",
		"|_|  |_/_/ \\_\\____|___|",
	},
	"samsung": {
		" ___   _   __  __ ___ _  _ _  _  ___ ",
		"/ __| /_\\ |  \\/  / __| | | | \\| |/ __|",
		"\\__ \\/ _ \\| |\\/| \\__ \\ |_| | .` | (_ |",
		"|___/_/ \\_\\_|  |_|___/\\___/|_|\\_|\\___|",
	},
	"apple": {
		"   _   ___ ___ _    ___ ",
		"  /_\\ | _ \\ _ \\ |  | __|",
		" / _ \\|  _/  _/ |__| _| ",
		"/_/ \\_\\_| |_| |____|___|",
	},
	"generic": {
		"  ____ ____  _   _ ",
		" / ___|  _ \\| | | |",
		"| |  _| |_) | | | |",
		"| |_| |  __/| |_| |",
		" \\____|_|    \\___/ ",
	},
}

// DisplayGPUInfo formats and prints detailed GPU and graphics API information (exported).
func DisplayGPUInfo(info *gather.GPUDetails, theme Theme) {
	vendorKey := strings.ToLower(info.Vendor)

	if theme != BlankTheme {
		switch vendorKey {
		case "nvidia":
			theme = Theme{
				Category: "\033[32m",       // Green
				Key:      "\033[38;5;82m",  // Bright Green
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;82m",  // Bright Green
				Reset:    "\033[0m",
			}
		case "amd":
			theme = Theme{
				Category: "\033[31m",       // Red
				Key:      "\033[38;5;196m", // Bright Red
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;196m", // Bright Red
				Reset:    "\033[0m",
			}
		case "intel":
			theme = Theme{
				Category: "\033[34m",       // Blue
				Key:      "\033[38;5;33m",  // Intel Blue
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;33m",  // Intel Blue
				Reset:    "\033[0m",
			}
		case "qualcomm":
			theme = Theme{
				Category: "\033[31m",       // Red/Orange
				Key:      "\033[38;5;208m", // Qualcomm Orange
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;208m", // Qualcomm Orange
				Reset:    "\033[0m",
			}
		case "arm":
			theme = Theme{
				Category: "\033[36m",       // Cyan
				Key:      "\033[38;5;51m",  // Bright Cyan
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;51m",  // Bright Cyan
				Reset:    "\033[0m",
			}
		case "samsung":
			theme = Theme{
				Category: "\033[34m",       // Blue
				Key:      "\033[38;5;39m",  // Samsung Blue
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;39m",  // Samsung Blue
				Reset:    "\033[0m",
			}
		case "apple":
			theme = Theme{
				Category: "\033[37m",       // White/Silver
				Key:      "\033[38;5;255m", // Bright White
				Value:    "\033[38;5;249m", // Light Gray
				Accent:   "\033[38;5;255m", // Bright White
				Reset:    "\033[0m",
			}
		default:
			distroKey := getDistroKey()
			theme = GetThemeForDistro(distroKey, false)
		}
	}

	termWidth := getTerminalWidth()
	logoWidth := 0

	var logoLines []string
	var exists bool
	logoLines, exists = GPULogos[vendorKey]
	if !exists {
		logoLines = GPULogos["generic"]
	}

	for _, l := range logoLines {
		runesCount := utf8.RuneCountInString(l)
		if runesCount > logoWidth {
			logoWidth = runesCount
		}
	}

	var infoLines []string

	username := getUsername()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)) + theme.Reset
	infoLines = append(infoLines, separator)

	items := []struct{ Key, Value string }{
		{"GPU Model", info.Name},
		{"Vendor", info.Vendor},
		{"Driver", info.Driver},
		{"VRAM Used", info.VRAMUsed},
		{"VRAM Total", info.VRAMTotal},
		{"VRAM Free", info.VRAMFree},
		{"Temperature", info.Temperature},
		{"OpenGL", info.OpenGL},
		{"Vulkan", info.Vulkan},
		{"OpenCL", info.OpenCL},
		{"CUDA", info.CUDA},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "Unknown" && val != "None" && val != "N/A" {
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}
			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	// Draw side-by-side or stacked layout using the vendor logo
	sideBySide := termWidth >= logoWidth+45
	if sideBySide {
		maxLines := len(logoLines)
		if len(infoLines) > maxLines {
			maxLines = len(infoLines)
		}

		for i := 0; i < maxLines; i++ {
			logoLine := ""
			runeLen := 0
			if i < len(logoLines) {
				logoLine = theme.Accent + logoLines[i] + theme.Reset
				runeLen = utf8.RuneCountInString(logoLines[i])
			} else {
				logoLine = strings.Repeat(" ", logoWidth)
			}

			if runeLen < logoWidth && i < len(logoLines) {
				logoLine += strings.Repeat(" ", logoWidth-runeLen)
			}

			infoLine := ""
			if i < len(infoLines) {
				infoLine = infoLines[i]
			}

			fmt.Printf("%s   %s\n", logoLine, infoLine)
		}
	} else {
		for _, l := range logoLines {
			fmt.Println(theme.Accent + l + theme.Reset)
		}
		fmt.Println()
		for _, l := range infoLines {
			fmt.Println(l)
		}
	}
}

// DisplayProcessList formats and prints the process list (Wider & Centered Title).
func DisplayProcessList(processList []gather.ProcessInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	pidWidth := 8
	cpuWidth := 8
	ramWidth := 8

	sideBySide := termWidth >= logoWidth+45

	var rightPanelWidth int
	if sideBySide {
		rightPanelWidth = termWidth - logoWidth - 3
	} else {
		rightPanelWidth = termWidth
	}

	nameWidth := rightPanelWidth - pidWidth - cpuWidth - ramWidth - 5
	if nameWidth < 15 {
		nameWidth = 15
	}

	totalWidth := pidWidth + nameWidth + cpuWidth + ramWidth + 3
	if totalWidth > rightPanelWidth {
		nameWidth -= (totalWidth - rightPanelWidth)
	}
	if nameWidth < 15 {
		nameWidth = 15
	}
	totalWidth = pidWidth + nameWidth + cpuWidth + ramWidth + 3

	var infoLines []string

	username := getUsername()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s (Processes)", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)+12) + theme.Reset
	infoLines = append(infoLines, separator)

	tableHeader := fmt.Sprintf("%s%-*s %-*s %*s %*s%s",
		theme.Key, pidWidth, "PID", nameWidth, "NAME", cpuWidth, "CPU%", ramWidth, "RAM", theme.Reset)
	infoLines = append(infoLines, tableHeader)

	tableSep := theme.Value + strings.Repeat("─", totalWidth) + theme.Reset
	infoLines = append(infoLines, tableSep)

	limit := 18
	if len(processList) < limit {
		limit = len(processList)
	}

	for _, p := range processList[:limit] {
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

		cpuColor := theme.Value
		if p.CPU > 20.0 {
			cpuColor = "\033[1;31m" // Bold Red
		} else if p.CPU > 5.0 {
			cpuColor = "\033[1;33m" // Bold Yellow
		}

		ramColor := theme.Value
		if p.RAM > 500*1024*1024 {
			ramColor = "\033[1;31m"
		} else if p.RAM > 150*1024*1024 {
			ramColor = "\033[1;33m"
		}

		row := fmt.Sprintf("%s%-*d %s%-*s %s%*.1f%% %s%*s%s",
			theme.Value, pidWidth, p.PID,
			theme.Key, nameWidth, truncateString(p.Name, nameWidth),
			cpuColor, cpuWidth-1, p.CPU,
			ramColor, ramWidth, ramStr,
			theme.Reset)
		infoLines = append(infoLines, row)
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// DisplayNetworkInfo formats and prints detailed network info (Exported)
func DisplayNetworkInfo(info *gather.NetworkInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	var infoLines []string

	username := getUsername()
	hostname := info.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s (Network)", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)+10) + theme.Reset
	infoLines = append(infoLines, separator)

	items := []struct{ Key, Value string }{
		{"Hostname", info.Hostname},
		{"Private IP", info.PrivateIP},
		{"MAC Address", info.MACAddress},
		{"I/O Counters", info.IOCounters},
		{"Public IP", info.PublicIP},
		{"ISP", info.ISP},
		{"Location", fmt.Sprintf("%s, %s", info.City, info.Country)},
		{"Proxy", info.Proxy},
		{"DNS Servers", strings.Join(info.DNSServers, ", ")},
		{"Ping (1.1.1.1)", info.Ping},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "N/A" && val != "Error" && val != "," && val != "Error, Error" {
			if item.Key == "Location" && (info.City == "" || info.City == "Error" || info.City == "Unknown") && (info.Country == "" || info.Country == "Error" || info.Country == "Unknown") {
				continue
			}
			if item.Key == "Location" {
				if info.City == "" || info.City == "Error" || info.City == "Unknown" {
					val = info.Country
				} else if info.Country == "" || info.Country == "Error" || info.Country == "Unknown" {
					val = info.City
				}
			}

			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}

			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// PrintHelp prints a distro-themed, highly aesthetic help screen to stdout.
func PrintHelp(version string, noColor bool) {
	distroKey := getDistroKey()
	theme := GetThemeForDistro(distroKey, noColor)

	title := " KernelView Go - System Fetch & Live Dashboard "
	verStr := fmt.Sprintf(" Version: %s (Detected: %s)", version, distroKey)

	// Print visual box header themed with distro primary colors
	fmt.Printf("%s╭────────────────────────────────────────────────────────╮%s\n", theme.Accent, theme.Reset)
	fmt.Printf("%s│%s%s%s%s%s│%s\n",
		theme.Accent,
		theme.Reset,
		"\033[1m",
		fmt.Sprintf("  %-54s", title),
		theme.Reset,
		theme.Accent,
		theme.Reset,
	)
	fmt.Printf("%s│%s%s%s%s│%s\n",
		theme.Accent,
		theme.Reset,
		theme.Value,
		fmt.Sprintf("  %-54s", verStr),
		theme.Accent,
		theme.Reset,
	)
	fmt.Printf("%s╰────────────────────────────────────────────────────────╯%s\n\n", theme.Accent, theme.Reset)

	// Section 1: Usage
	fmt.Printf("%sUSAGE:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  kernelview [flags]\n\n")

	// Section 2: Display Modes
	fmt.Printf("%sDISPLAY MODES:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-l, --live%s         %sStart real-time TUI dashboard (interactive, multi-tab)%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-p, --process%s      %sDisplay static list of running processes%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-n, --network%s      %sDisplay detailed static network interfaces & stats%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-g, --gpu%s          %sDisplay detailed static GPU & graphics API report%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s[default]%s          %sDisplay system fetch (fastfetch-like distro ASCII logo & specs)%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 3: Configuration Options
	fmt.Printf("%sCONFIGURATION OPTIONS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-f, --fast%s         %sRun system fetch in fast mode (skips slow subsystem checks)%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s--json%s             %sOutput information as structured JSON%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s--no-color%s         %sDisable all ANSI color formatting%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 4: Information Flags
	fmt.Printf("%sINFO FLAGS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-v, --version%s      %sPrint version and build information%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-h, --help%s         %sPrint this aesthetic help menu%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 5: TUI Interactive Keys
	fmt.Printf("%sTUI INTERACTIVE KEYS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s[1]%s Dashboard Tab   %s[2]%s Processes Tab   %s[3]%s Network Tab   %s[4]%s CPU Cores Tab   %s[Q]%s Quit\n\n",
		theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset)

	// Section 6: Examples
	fmt.Printf("%sEXAMPLES:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  $ kernelview --live            %s# Open the live TUI dashboard%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -p --json         %s# Fetch running processes in JSON format%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -g                %s# Display detailed GPU and graphics API report%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -f                %s# Fast system fetch with ASCII logo%s\n\n", "\033[90m", theme.Reset)

	// Footer with Color Grid blocks
	blocks := getColorBlocks(theme)
	if blocks != "" {
		fmt.Printf("%s\n\n", blocks)
	}
}
