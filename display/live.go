package display

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"KernelView-Go/gather"
	"golang.org/x/term"
)

type EventType int

const (
	Tick EventType = iota
	Key
)

type Event struct {
	Type EventType
	Char byte
}

type LiveDisplay struct {
	tracker    *gather.LiveTracker
	static     *gather.SystemInfo
	isFast     bool
	tab        string
	termWidth  int
	termHeight int
	peakRx     float64
	peakTx     float64
}

// StartLiveDashboard launches the clean, aesthetic live dashboard
func StartLiveDashboard(isFast bool) {
	fd := int(os.Stdin.Fd())
	var oldState *term.State
	var errRaw error

	restored := false
	cleanup := func() {
		if !restored {
			restored = true
			if oldState != nil {
				_ = term.Restore(fd, oldState)
			}
			fmt.Print("\033[?25h\033[?1049l\033[0m") // Show cursor, exit alternate screen, reset
		}
	}
	defer cleanup()

	defer func() {
		if r := recover(); r != nil {
			cleanup()
			fmt.Printf("\r\nKernelView encountered an error: %v\r\n", r)
		}
	}()

	// Alternate screen buffer, clear screen, home cursor, hide cursor
	fmt.Print("\033[?1049h\033[2J\033[H\033[?25l")

	// Set raw mode
	oldState, errRaw = term.MakeRaw(fd)
	if errRaw != nil {
		oldState = nil
	}

	// Trap signals to cleanly restore terminal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		cleanup()
		os.Exit(0)
	}()

	events := make(chan Event)
	tracker := gather.NewLiveTracker()
	staticInfo := gather.GetSystemInfo(true)
	staticInfo.Packages = "Loading..."

	// Background package count resolver
	go func() {
		defer func() { _ = recover() }()
		fullInfo := gather.GetSystemInfo(false)
		if fullInfo != nil && fullInfo.Packages != "" && fullInfo.Packages != "N/A" {
			staticInfo.Packages = fullInfo.Packages
		}
	}()

	// Input reader
	go func() {
		defer func() { _ = recover() }()
		var buf [1]byte
		for {
			n, err := os.Stdin.Read(buf[:])
			if err != nil || n == 0 {
				return
			}
			events <- Event{Type: Key, Char: buf[0]}
		}
	}()

	// Live ticker (350ms with anti-jitter damping)
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		defer func() { _ = recover() }()
		for range ticker.C {
			events <- Event{Type: Tick}
		}
	}()

	ld := &LiveDisplay{
		tracker: tracker,
		static:  staticInfo,
		isFast:  isFast,
		tab:     "dashboard",
	}

	ld.updateSize()
	ld.render()

	for ev := range events {
		if ev.Type == Key {
			char := ev.Char
			if char == 'q' || char == 'Q' || char == 3 { // 3 = Ctrl+C
				break
			}
			switch char {
			case '1', 'd', 'D':
				ld.tab = "dashboard"
				fmt.Print("\033[H\033[2J")
			case '2', 'p', 'P':
				ld.tab = "processes"
				fmt.Print("\033[H\033[2J")
			case '3', 'n', 'N':
				ld.tab = "network"
				fmt.Print("\033[H\033[2J")
			case '4', 'c', 'C':
				ld.tab = "cores"
				fmt.Print("\033[H\033[2J")
			}
		}

		ld.updateSize()
		ld.render()
	}
}

func (ld *LiveDisplay) updateSize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && w > 0 && h > 0 {
		if w != ld.termWidth || h != ld.termHeight {
			ld.termWidth = w
			ld.termHeight = h
			fmt.Print("\033[H\033[2J") // Clear on resize
		}
	} else {
		if ld.termWidth == 0 {
			ld.termWidth = 80
			ld.termHeight = 24
		}
	}
}

func (ld *LiveDisplay) renderWarningScreen(theme Theme) {
	fmt.Print("\033[H\033[2J")
	boxWidth := 46
	if ld.termWidth < boxWidth {
		boxWidth = ld.termWidth - 2
		if boxWidth < 10 {
			boxWidth = 10
		}
	}

	lines := []string{
		"",
		"  ⚠️   Terminal Size Too Constrained",
		"",
		fmt.Sprintf("  Current Size: %d x %d", ld.termWidth, ld.termHeight),
		"  Required Min: 60 x 15",
		"",
		"  Please enlarge your terminal window",
		"  or decrease font zoom to restore dashboard.",
		"",
		"  Press [Q] to quit.",
	}

	box := drawBox("Warning", lines, boxWidth, len(lines)+2, theme)
	padY := (ld.termHeight - len(box)) / 2
	for i := 0; i < padY; i++ {
		fmt.Print("\033[K\r\n")
	}
	for _, l := range box {
		padX := (ld.termWidth - boxWidth) / 2
		if padX < 0 {
			padX = 0
		}
		fmt.Printf("%s%s\033[K\r\n", strings.Repeat(" ", padX), l)
	}
	fmt.Print("\033[J")
}

func (ld *LiveDisplay) render() {
	distroKey := getDistroKey()
	theme := GetThemeForDistro(distroKey, false)

	if ld.termWidth < 60 || ld.termHeight < 15 {
		ld.renderWarningScreen(theme)
		return
	}

	metrics, err := ld.tracker.GetMetrics()
	if err != nil {
		return
	}

	// Update peak network rates
	if metrics.NetRxSpeed > ld.peakRx {
		ld.peakRx = metrics.NetRxSpeed
	}
	if metrics.NetTxSpeed > ld.peakTx {
		ld.peakTx = metrics.NetTxSpeed
	}

	// Move cursor to home (top-left) to redraw cleanly
	fmt.Print("\033[H")

	headerLines := ld.printHeader(theme)
	footerLines := 1
	if ld.termHeight >= 26 {
		footerLines = 2
	}
	availableHeight := ld.termHeight - headerLines - footerLines
	if availableHeight < 10 {
		availableHeight = 10
	}

	switch ld.tab {
	case "processes":
		ld.renderProcesses(metrics, theme, availableHeight)
	case "network":
		ld.renderNetwork(metrics, theme, availableHeight)
	case "cores":
		ld.renderCores(metrics, theme, availableHeight)
	default:
		ld.renderDashboard(metrics, theme, availableHeight)
	}

	fmt.Print("\033[J")
	ld.printFooter(theme, footerLines == 2)
}

// -------------------------------------------------------------
// Header & Footer
// -------------------------------------------------------------

func (ld *LiveDisplay) printHeader(theme Theme) int {
	w := ld.termWidth

	// 1. Top Brand Banner with Distro & Clock
	nowStr := time.Now().Format("15:04:05")
	brand := " ◈ KERNELVIEW LIVE ◈ "

	var metaStr string
	if w >= 85 {
		metaStr = fmt.Sprintf(" %s │ %s │ %s ", ld.static.OS, ld.static.Kernel, nowStr)
	} else if w >= 65 {
		metaStr = fmt.Sprintf(" %s │ %s ", ld.static.OS, nowStr)
	} else {
		metaStr = fmt.Sprintf(" %s ", nowStr)
	}

	brandLen := visualLen(brand)
	metaLen := visualLen(metaStr)
	fillLen := w - brandLen - metaLen
	if fillLen < 0 {
		metaStr = fmt.Sprintf(" %s ", nowStr)
		metaLen = visualLen(metaStr)
		fillLen = w - brandLen - metaLen
		if fillLen < 0 {
			metaStr = ""
			fillLen = w - brandLen
		}
	}
	if fillLen < 0 {
		fillLen = 0
	}

	topBanner := fmt.Sprintf("%s\033[1;7m%s\033[0m\033[38;5;238m%s\033[0m\033[38;5;244m%s\033[0m\033[K\r\n",
		theme.Accent, brand, strings.Repeat("─", fillLen), metaStr)
	fmt.Print(topBanner)

	// 2. Navigation Tabs (Pill Buttons)
	tabNames := []struct {
		id   string
		name string
	}{
		{"dashboard", "1. DASHBOARD"},
		{"processes", "2. PROCESSES"},
		{"network", "3. NETWORK"},
		{"cores", "4. CPU CORES"},
	}

	var tabStrs []string
	for _, t := range tabNames {
		if ld.tab == t.id {
			tabStrs = append(tabStrs, fmt.Sprintf("%s\033[1;7m  %s  \033[0m", theme.Accent, t.name))
		} else {
			tabStrs = append(tabStrs, fmt.Sprintf("\033[90m[ \033[37m%s\033[90m ]\033[0m", t.name))
		}
	}

	tabsLine := "  " + strings.Join(tabStrs, "  ")
	if ld.termHeight >= 28 {
		fmt.Printf("\033[K\r\n%s\033[K\r\n\033[K\r\n", tabsLine)
		return 4
	} else if ld.termHeight >= 22 {
		fmt.Printf("%s\033[K\r\n\033[K\r\n", tabsLine)
		return 3
	} else {
		fmt.Printf("%s\033[K\r\n", tabsLine)
		return 2
	}
}

func (ld *LiveDisplay) printFooter(theme Theme, withLeadingNewline bool) {
	keyQ := fmt.Sprintf("%s\033[1;7m Q \033[0m", theme.Accent)
	key1 := fmt.Sprintf("%s\033[1;7m 1 \033[0m", theme.Accent)
	key2 := fmt.Sprintf("%s\033[1;7m 2 \033[0m", theme.Accent)
	key3 := fmt.Sprintf("%s\033[1;7m 3 \033[0m", theme.Accent)
	key4 := fmt.Sprintf("%s\033[1;7m 4 \033[0m", theme.Accent)

	leftPart := fmt.Sprintf(" %s Quit   %s Dashboard   %s Processes   %s Network   %s Cores", keyQ, key1, key2, key3, key4)
	rightPart := "\033[90mRefresh: 350ms │ KernelView \033[0m"

	leftLen := visualLen(leftPart)
	rightLen := visualLen(rightPart)
	fillLen := ld.termWidth - leftLen - rightLen - 1
	if fillLen < 1 {
		fillLen = 1
	}

	footerText := fmt.Sprintf("%s%s%s", leftPart, strings.Repeat(" ", fillLen), rightPart)

	if withLeadingNewline {
		fmt.Printf("\033[K\r\n%s\033[K", footerText)
	} else {
		fmt.Printf("%s\033[K", footerText)
	}
}

// -------------------------------------------------------------
// Tab 1: Dashboard
// -------------------------------------------------------------

func (ld *LiveDisplay) renderDashboard(metrics *gather.LiveMetrics, theme Theme, availableHeight int) {
	isDualCol := ld.termWidth >= 80 && availableHeight >= 14

	if !isDualCol {
		ld.renderDashboardSingleCol(metrics, theme, availableHeight)
		return
	}

	// Dual Column Mode
	leftW := (ld.termWidth - 3) / 2
	rightW := ld.termWidth - leftW - 3

	// --- Left Column: Box 1 (System & Hardware) + Box 2 (Network Info) ---
	sysBoxH := (availableHeight * 11) / 20
	if sysBoxH < 8 {
		sysBoxH = 8
	}
	netBoxH := availableHeight - sysBoxH

	wmDe := ld.static.WindowManager
	if ld.static.DE != "" {
		if wmDe != "" && wmDe != ld.static.DE {
			wmDe = fmt.Sprintf("%s (%s)", wmDe, ld.static.DE)
		} else {
			wmDe = ld.static.DE
		}
	}
	if wmDe == "" {
		wmDe = "N/A"
	}

	sysLines := []string{
		formatKeyVal("OS", ld.static.OS, leftW-4, theme),
		formatKeyVal("Host", ld.static.Host, leftW-4, theme),
		formatKeyVal("Kernel", ld.static.Kernel, leftW-4, theme),
		formatKeyVal("Uptime", metrics.Uptime, leftW-4, theme),
		formatKeyVal("CPU", ld.static.CPU, leftW-4, theme),
		formatKeyVal("GPU", ld.static.GPU, leftW-4, theme),
		formatKeyVal("Memory", ld.static.RAM, leftW-4, theme),
		formatKeyVal("DE / WM", wmDe, leftW-4, theme),
		formatKeyVal("Resolution", ld.static.Resolution, leftW-4, theme),
		formatKeyVal("Shell / Term", fmt.Sprintf("%s / %s", ld.static.Shell, ld.static.Terminal), leftW-4, theme),
		formatKeyVal("Packages", ld.static.Packages, leftW-4, theme),
		formatKeyVal("Local IP", ld.static.IPAddress, leftW-4, theme),
		formatKeyVal("Virt / Locale", fmt.Sprintf("%s / %s", ld.static.Virtualization, ld.static.Locale), leftW-4, theme),
	}
	sysBox := drawBox("◆ System & Hardware", sysLines, leftW, sysBoxH, theme)

	// Clean Network Information Box
	var netLines []string
	netLines = append(netLines, formatKeyVal("Active Interface", metrics.NetIface, leftW-4, theme))
	netLines = append(netLines, formatKeyVal("Local IP Address", ld.static.IPAddress, leftW-4, theme))
	netLines = append(netLines, formatKeyVal("Download Speed", fmt.Sprintf("▼ %s/s (Peak: %s/s)", gather.FormatBytes(uint64(metrics.NetRxSpeed)), gather.FormatBytes(uint64(ld.peakRx))), leftW-4, theme))
	netLines = append(netLines, formatKeyVal("Upload Speed", fmt.Sprintf("▲ %s/s (Peak: %s/s)", gather.FormatBytes(uint64(metrics.NetTxSpeed)), gather.FormatBytes(uint64(ld.peakTx))), leftW-4, theme))
	netLines = append(netLines, formatKeyVal("Total Download", fmt.Sprintf("▼ %s", gather.FormatBytes(metrics.NetRxTotal)), leftW-4, theme))
	netLines = append(netLines, formatKeyVal("Total Upload", fmt.Sprintf("▲ %s", gather.FormatBytes(metrics.NetTxTotal)), leftW-4, theme))

	netBox := drawBox("▲ Network Activity & IO", netLines, leftW, netBoxH, theme)

	leftCol := append(sysBox, netBox...)

	// --- Right Column: Box 3 (Resource Monitor) + Box 4 (Top Processes) ---
	resBoxH := 8
	if metrics.GPUMetrics.HasGPU {
		resBoxH = 9
	}
	if metrics.SwapTotal > 0 {
		resBoxH++
	}
	procBoxH := availableHeight - resBoxH

	// Resource Monitor Lines with Clean Progress Bars
	barW := rightW - 28
	if barW > 26 {
		barW = 26
	}
	if barW < 10 {
		barW = 10
	}

	var resLines []string
	cpuTempStr := "N/A"
	if metrics.Temperature > 0 {
		cpuTempStr = formatTemp(metrics.Temperature)
	}

	resLines = append(resLines, fmt.Sprintf("%-10s %s  %s",
		fmt.Sprintf("%sCPU Usage%s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
		drawCleanBar(metrics.CPUUsage, barW),
		cpuTempStr))

	resLines = append(resLines, fmt.Sprintf("%-10s %s  \033[38;5;244m%s / %s\033[0m",
		fmt.Sprintf("%sRAM Usage%s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
		drawCleanBar(metrics.RAMPercent, barW),
		gather.FormatBytes(metrics.RAMUsed), gather.FormatBytes(metrics.RAMTotal)))

	if metrics.SwapTotal > 0 {
		resLines = append(resLines, fmt.Sprintf("%-10s %s  \033[38;5;244m%s / %s\033[0m",
			fmt.Sprintf("%sSwap     %s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
			drawCleanBar(metrics.SwapPercent, barW),
			gather.FormatBytes(metrics.SwapUsed), gather.FormatBytes(metrics.SwapTotal)))
	}

	resLines = append(resLines, fmt.Sprintf("%-10s %s  \033[38;5;244m%s / %s\033[0m",
		fmt.Sprintf("%sDisk (/) %s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
		drawCleanBar(metrics.DiskPercent, barW),
		gather.FormatBytes(metrics.DiskUsed), gather.FormatBytes(metrics.DiskTotal)))

	if metrics.GPUMetrics.HasGPU {
		gpuTempStr := "N/A"
		if metrics.GPUMetrics.GPUTemp > 0 {
			gpuTempStr = formatTemp(metrics.GPUMetrics.GPUTemp)
		}
		resLines = append(resLines, fmt.Sprintf("%-10s %s  %s",
			fmt.Sprintf("%sGPU Usage%s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
			drawCleanBar(metrics.GPUMetrics.GPUUsage, barW),
			gpuTempStr))
	}

	resBox := drawBox("◈ Resource Monitor", resLines, rightW, resBoxH, theme)

	// Top Processes Lines
	var procLines []string
	nameW := rightW - 28
	if nameW < 8 {
		nameW = 8
	}

	procLines = append(procLines, fmt.Sprintf("%s%-6s %-*s %7s %7s%s", theme.Key, "PID", nameW, "COMMAND", "CPU%", "RAM", theme.Reset))
	procLines = append(procLines, "\033[90m"+strings.Repeat("─", rightW-4)+theme.Reset)

	maxProcs := procBoxH - 4
	if maxProcs < 1 {
		maxProcs = 1
	}

	actualCount := len(metrics.Processes)
	if actualCount > maxProcs {
		actualCount = maxProcs
	}

	for i := 0; i < actualCount; i++ {
		p := metrics.Processes[i]
		cpuColor := "\033[38;5;250m"
		if p.CPU >= 50.0 {
			cpuColor = "\033[1;38;5;196m" // Bold Red
		} else if p.CPU >= 15.0 {
			cpuColor = "\033[1;38;5;220m" // Amber
		} else if p.CPU >= 5.0 {
			cpuColor = "\033[38;5;48m" // Green
		}

		ramStr := formatShortBytes(p.RAM)
		row := fmt.Sprintf("\033[38;5;45m%-6d\033[0m %-*s %s%6.1f%%\033[0m %7s",
			p.PID, nameW, truncateAnsi(p.Name, nameW), cpuColor, p.CPU, ramStr)
		procLines = append(procLines, row)
	}

	procBox := drawBox("⬢ Top Processes", procLines, rightW, procBoxH, theme)

	rightCol := append(resBox, procBox...)

	printSideBySide(leftCol, rightCol, 2)
}

func (ld *LiveDisplay) renderDashboardSingleCol(metrics *gather.LiveMetrics, theme Theme, availableHeight int) {
	w := ld.termWidth - 2
	if w < 40 {
		w = 40
	}

	var allRows []string

	// 1. Resource Monitor
	barW := 16
	var resLines []string
	resLines = append(resLines, fmt.Sprintf("%-10s %s", "CPU Usage:", drawCleanBar(metrics.CPUUsage, barW)))
	resLines = append(resLines, fmt.Sprintf("%-10s %s", "RAM Usage:", drawCleanBar(metrics.RAMPercent, barW)))
	resLines = append(resLines, formatKeyVal("RAM Bytes", fmt.Sprintf("%s / %s", gather.FormatBytes(metrics.RAMUsed), gather.FormatBytes(metrics.RAMTotal)), w-4, theme))
	resLines = append(resLines, fmt.Sprintf("%-10s %s", "Disk (/):", drawCleanBar(metrics.DiskPercent, barW)))

	resH := len(resLines) + 2
	resBox := drawBox("◈ Resource Monitor", resLines, w, resH, theme)
	allRows = append(allRows, resBox...)

	// 2. Processes
	remH := availableHeight - len(allRows)
	if remH >= 6 {
		nameW := w - 24
		if nameW < 8 {
			nameW = 8
		}
		var procLines []string
		procLines = append(procLines, fmt.Sprintf("%s%-6s %-*s %6s %7s%s", theme.Key, "PID", nameW, "COMMAND", "CPU%", "RAM", theme.Reset))
		procLines = append(procLines, "\033[90m"+strings.Repeat("─", w-4)+theme.Reset)

		maxProcs := remH - 4
		if maxProcs > len(metrics.Processes) {
			maxProcs = len(metrics.Processes)
		}
		for i := 0; i < maxProcs; i++ {
			p := metrics.Processes[i]
			cpuColor := "\033[38;5;250m"
			if p.CPU >= 20.0 {
				cpuColor = "\033[1;38;5;196m"
			} else if p.CPU >= 5.0 {
				cpuColor = "\033[1;38;5;220m"
			}
			row := fmt.Sprintf("\033[38;5;45m%-6d\033[0m %-*s %s%5.1f%%\033[0m %7s",
				p.PID, nameW, truncateAnsi(p.Name, nameW), cpuColor, p.CPU, formatShortBytes(p.RAM))
			procLines = append(procLines, row)
		}
		procBox := drawBox("⬢ Top Processes", procLines, w, remH, theme)
		allRows = append(allRows, procBox...)
	}

	for _, l := range allRows {
		fmt.Printf("%s\033[K\r\n", l)
	}
}

// -------------------------------------------------------------
// Tab 2: Processes (Full Height & Full Width Table)
// -------------------------------------------------------------

func (ld *LiveDisplay) renderProcesses(metrics *gather.LiveMetrics, theme Theme, availableHeight int) {
	w := ld.termWidth - 2
	if w < 50 {
		w = 50
	}

	pidW := 8
	userW := 10
	cpuW := 16
	ramPercW := 16
	ramByteW := 10
	nameW := w - pidW - userW - cpuW - ramPercW - ramByteW - 8
	if nameW < 12 {
		nameW = 12
	}

	var procLines []string
	header := fmt.Sprintf("%s%-*s %-*s %-*s %*s %*s %*s%s",
		theme.Key, pidW, "PID", userW, "USER", nameW, "COMMAND / PROCESS", cpuW, "CPU USAGE", ramPercW, "MEM USAGE", ramByteW, "RAM", theme.Reset)
	procLines = append(procLines, header)
	procLines = append(procLines, "\033[90m"+strings.Repeat("─", w-4)+theme.Reset)

	maxRows := availableHeight - 4
	if maxRows < 1 {
		maxRows = 1
	}

	totalProcs := len(metrics.Processes)
	count := totalProcs
	if count > maxRows {
		count = maxRows
	}

	for i := 0; i < count; i++ {
		p := metrics.Processes[i]
		cpuBar := drawMiniBar(p.CPU, 6)
		ramBar := drawMiniBar(float64(p.RAMPerc), 6)

		userStr := "root"
		if p.PID > 100 {
			userStr = ld.static.Host
			if len(userStr) > userW {
				userStr = userStr[:userW]
			}
		}

		cpuText := fmt.Sprintf("%5.1f%% %s", p.CPU, cpuBar)
		ramText := fmt.Sprintf("%5.1f%% %s", p.RAMPerc, ramBar)

		row := fmt.Sprintf("\033[38;5;45m%-*d\033[0m \033[38;5;244m%-*s\033[0m %-*s %*s %*s \033[38;5;250m%*s\033[0m",
			pidW, p.PID,
			userW, userStr,
			nameW, truncateAnsi(p.Name, nameW),
			cpuW, cpuText,
			ramPercW, ramText,
			ramByteW, formatShortBytes(p.RAM))
		procLines = append(procLines, row)
	}

	box := drawBox(fmt.Sprintf("⬢ Process Activity Monitor (%d total processes)", totalProcs), procLines, w, availableHeight, theme)
	for _, l := range box {
		fmt.Printf("%s\033[K\r\n", l)
	}
}

// -------------------------------------------------------------
// Tab 3: Network (Clean Summary & Interfaces Table)
// -------------------------------------------------------------

func (ld *LiveDisplay) renderNetwork(metrics *gather.LiveMetrics, theme Theme, availableHeight int) {
	w := ld.termWidth - 2
	if w < 50 {
		w = 50
	}

	var netLines []string

	// 1. Network Metadata Header Cards
	netLines = append(netLines, formatKeyVal("Hostname", ld.static.Hostname, w-4, theme))
	netLines = append(netLines, formatKeyVal("Active Interface", fmt.Sprintf("%s (IP: %s)", metrics.NetIface, ld.static.IPAddress), w-4, theme))
	netLines = append(netLines, formatKeyVal("Download Speed", fmt.Sprintf("▼ %s/s (Peak: %s/s)", gather.FormatBytes(uint64(metrics.NetRxSpeed)), gather.FormatBytes(uint64(ld.peakRx))), w-4, theme))
	netLines = append(netLines, formatKeyVal("Upload Speed", fmt.Sprintf("▲ %s/s (Peak: %s/s)", gather.FormatBytes(uint64(metrics.NetTxSpeed)), gather.FormatBytes(uint64(ld.peakTx))), w-4, theme))
	netLines = append(netLines, formatKeyVal("Total Traffic", fmt.Sprintf("▼ Download: %s   ▲ Upload: %s", gather.FormatBytes(metrics.NetRxTotal), gather.FormatBytes(metrics.NetTxTotal)), w-4, theme))
	netLines = append(netLines, "")

	// 2. Network Interfaces Table
	ifaceColW := 18
	speedColW := 14
	netLines = append(netLines, fmt.Sprintf("%s%-*s %-*s %*s %*s%s", theme.Key, ifaceColW, "INTERFACE", 12, "STATE", speedColW, "RX SPEED", speedColW, "TX SPEED", theme.Reset))
	netLines = append(netLines, "\033[90m"+strings.Repeat("─", w-4)+theme.Reset)

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 && iface.Flags&net.FlagLoopback == 0 {
				continue
			}
			stateStr := "\033[38;5;48mUP\033[0m"
			if iface.Flags&net.FlagUp == 0 {
				stateStr = "\033[38;5;196mDOWN\033[0m"
			}
			name := iface.Name
			if iface.Flags&net.FlagLoopback != 0 {
				name += " (lo)"
			}

			var rxS, txS string
			if iface.Name == metrics.NetIface {
				rxS = fmt.Sprintf("%s/s", gather.FormatBytes(uint64(metrics.NetRxSpeed)))
				txS = fmt.Sprintf("%s/s", gather.FormatBytes(uint64(metrics.NetTxSpeed)))
			} else {
				rxS = "0 B/s"
				txS = "0 B/s"
			}

			row := fmt.Sprintf("%-*s %-*s %*s %*s", ifaceColW, name, 12+9, stateStr, speedColW, rxS, speedColW, txS)
			netLines = append(netLines, row)
		}
	}

	box := drawBox("▲ Network Interfaces & Bandwidth Monitor", netLines, w, availableHeight, theme)
	for _, l := range box {
		fmt.Printf("%s\033[K\r\n", l)
	}
}

// -------------------------------------------------------------
// Tab 4: CPU Cores (Per-Core Thread Grid & Telemetry)
// -------------------------------------------------------------

func (ld *LiveDisplay) renderCores(metrics *gather.LiveMetrics, theme Theme, availableHeight int) {
	w := ld.termWidth - 2
	if w < 50 {
		w = 50
	}

	var coreLines []string

	// 1. CPU Hardware Info
	coreLines = append(coreLines, formatKeyVal("CPU Model", ld.static.CPU, w-4, theme))
	metaInfo := fmt.Sprintf("Threads: %d  │  Base Clock: %s  │  Temperature: %s",
		len(metrics.CPUCores), ld.static.CPUSpeed, formatTemp(metrics.Temperature))
	coreLines = append(coreLines, formatKeyVal("Topology", metaInfo, w-4, theme))
	coreLines = append(coreLines, fmt.Sprintf("%-16s %s",
		fmt.Sprintf("%sOverall CPU Load%s\033[90m:%s", theme.Key, theme.Reset, theme.Reset),
		drawCleanBar(metrics.CPUUsage, 24)))
	coreLines = append(coreLines, "")

	// 2. Grid for CPU Cores (2 or 4 Columns)
	numCores := len(metrics.CPUCores)
	if numCores > 0 {
		numCols := 2
		if w >= 110 && numCores >= 8 {
			numCols = 4
		}

		colW := (w - 4 - (numCols-1)*3) / numCols
		if colW < 20 {
			colW = 20
			numCols = 2
		}

		barW := colW - 18
		if barW < 8 {
			barW = 8
		}
		if barW > 24 {
			barW = 24
		}

		for i := 0; i < numCores; i += numCols {
			var colParts []string
			for c := 0; c < numCols; c++ {
				idx := i + c
				if idx < numCores {
					val := metrics.CPUCores[idx]
					bar := drawCleanBar(val, barW)
					part := fmt.Sprintf("%sCPU %-2d%s\033[90m:%s %s", theme.Key, idx, theme.Reset, theme.Reset, bar)
					colParts = append(colParts, part)
				} else {
					colParts = append(colParts, strings.Repeat(" ", colW))
				}
			}
			coreLines = append(coreLines, strings.Join(colParts, " \033[90m│\033[0m "))
		}
	} else {
		coreLines = append(coreLines, "No per-core CPU telemetry available.")
	}

	box := drawBox("■ CPU Cores Telemetry & Frequency", coreLines, w, availableHeight, theme)
	for _, l := range box {
		fmt.Printf("%s\033[K\r\n", l)
	}
}

// -------------------------------------------------------------
// Rendering & String Formatting Helpers
// -------------------------------------------------------------

func runeCellWidth(r rune) int {
	// Zero-width characters (variation selectors, combining marks)
	if (r >= 0xFE00 && r <= 0xFE0F) || (r >= 0x0300 && r <= 0x036F) {
		return 0
	}
	// Common wide characters / emojis (2 cells)
	if (r >= 0x1F000 && r <= 0x1FFFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2B50 && r <= 0x2B55) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0x303E) ||
		(r >= 0x3040 && r <= 0x33BF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) {
		return 2
	}
	// Standard single-width printable runes
	if r >= 0x20 {
		return 1
	}
	return 0
}

func visualLen(s string) int {
	stripped := stripAnsi(s)
	width := 0
	for _, r := range stripped {
		width += runeCellWidth(r)
	}
	return width
}

func truncateAnsi(s string, maxVisualLen int) string {
	if maxVisualLen <= 0 {
		return ""
	}
	if visualLen(s) <= maxVisualLen {
		return s
	}

	var sb strings.Builder
	currentLen := 0
	inEscape := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 0x1b { // ESC
			inEscape = true
			sb.WriteRune(r)
			continue
		}
		if inEscape {
			sb.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		w := runeCellWidth(r)
		if currentLen+w <= maxVisualLen {
			sb.WriteRune(r)
			currentLen += w
		} else {
			break
		}
	}
	sb.WriteString("\033[0m")
	return sb.String()
}

func padLine(s string, targetWidth int) string {
	vLen := visualLen(s)
	if vLen < targetWidth {
		return s + strings.Repeat(" ", targetWidth-vLen)
	}
	if vLen > targetWidth {
		return truncateAnsi(s, targetWidth)
	}
	return s
}

func drawBox(title string, lines []string, width int, height int, theme Theme) []string {
	var box []string

	borderColor := theme.Accent
	if borderColor == "" {
		borderColor = "\033[38;5;39m"
	}

	contentW := width - 4
	if contentW < 4 {
		contentW = 4
	}

	// 1. Top border with title
	titlePart := " " + title + " "
	if visualLen(titlePart) > contentW {
		titlePart = " " + truncateAnsi(title, contentW-2) + " "
	}
	topPrefix := borderColor + "╭─┤" + theme.Reset + "\033[1m" + titlePart + theme.Reset + borderColor + "├"
	prefixLen := visualLen(topPrefix)
	remTop := width - prefixLen - 1
	if remTop < 0 {
		remTop = 0
	}
	top := topPrefix + strings.Repeat("─", remTop) + "╮" + theme.Reset
	box = append(box, top)

	// 2. Content rows
	contentRows := height - 2
	if contentRows < 0 {
		contentRows = len(lines)
	}

	for i := 0; i < contentRows; i++ {
		var rowContent string
		if i < len(lines) {
			rowContent = lines[i]
		} else {
			rowContent = ""
		}
		padded := padLine(rowContent, contentW)
		box = append(box, borderColor+"│ "+theme.Reset+padded+borderColor+" │"+theme.Reset)
	}

	// 3. Bottom border
	bottomRem := width - 2
	if bottomRem < 0 {
		bottomRem = 0
	}
	bottom := borderColor + "╰" + strings.Repeat("─", bottomRem) + "╯" + theme.Reset
	box = append(box, bottom)

	return box
}

func printSideBySide(left []string, right []string, spacing int) {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}

	leftW := 0
	if len(left) > 0 {
		leftW = visualLen(left[0])
	}
	rightW := 0
	if len(right) > 0 {
		rightW = visualLen(right[0])
	}

	spacer := strings.Repeat(" ", spacing)

	for i := 0; i < max; i++ {
		var lPart string
		if i < len(left) {
			lPart = left[i]
		} else {
			lPart = strings.Repeat(" ", leftW)
		}

		var rPart string
		if i < len(right) {
			rPart = right[i]
		} else {
			rPart = strings.Repeat(" ", rightW)
		}

		fmt.Printf("%s%s%s\033[K\r\n", lPart, spacer, rPart)
	}
}

func formatKeyVal(key string, val string, width int, theme Theme) string {
	keyPart := fmt.Sprintf("%s%-16s%s\033[90m:%s", theme.Key, key, theme.Reset, theme.Reset)
	vLenKey := visualLen(keyPart)
	rem := width - vLenKey - 1
	if rem < 4 {
		rem = 4
	}
	valTrunc := truncateAnsi(val, rem)
	return padLine(fmt.Sprintf("%s \033[1;37m%s\033[0m", keyPart, valTrunc), width)
}

// drawCleanBar renders an ultra-clean, modern progress bar with sleek track
func drawCleanBar(percent float64, barW int) string {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}

	if barW < 4 {
		barW = 4
	}
	filled := int(float64(barW) * (percent / 100.0))
	empty := barW - filled

	var color string
	if percent >= 80 {
		color = "\033[1;38;5;196m" // Bold Red
	} else if percent >= 50 {
		color = "\033[1;38;5;220m" // Amber Gold
	} else {
		color = "\033[38;5;48m" // Bright Mint Green
	}

	filledStr := color + strings.Repeat("■", filled) + "\033[0m"
	emptyStr := "\033[90m" + strings.Repeat("─", empty) + "\033[0m"

	return fmt.Sprintf("\033[90m[%s%s\033[90m]\033[0m \033[1;37m%3.0f%%\033[0m", filledStr, emptyStr, percent)
}

// drawMiniBar draws a tiny inline progress bar for the process table
func drawMiniBar(percent float64, barW int) string {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	if barW < 4 {
		barW = 4
	}
	filled := int(float64(barW) * (percent / 100.0))
	empty := barW - filled

	var color string
	if percent >= 50 {
		color = "\033[1;38;5;196m"
	} else if percent >= 20 {
		color = "\033[1;38;5;220m"
	} else {
		color = "\033[38;5;48m"
	}

	return fmt.Sprintf("%s%s\033[90m%s\033[0m", color, strings.Repeat("■", filled), strings.Repeat("─", empty))
}

func formatTemp(t float64) string {
	color := "\033[38;5;48m" // Green
	if t >= 75 {
		color = "\033[1;38;5;196m" // Red
	} else if t >= 60 {
		color = "\033[1;38;5;220m" // Yellow
	}
	return fmt.Sprintf("%s%.1f °C\033[0m", color, t)
}

func formatShortBytes(b uint64) string {
	mb := float64(b) / (1024 * 1024)
	if mb < 1.0 {
		return fmt.Sprintf("%d B", b)
	}
	if mb < 1024 {
		return fmt.Sprintf("%.0fM", mb)
	}
	gb := mb / 1024
	return fmt.Sprintf("%.1fG", gb)
}
