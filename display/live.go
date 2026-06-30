package display

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
	"unicode/utf8"

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
}

func StartLiveDashboard(isFast bool) {
	// Switch to alternate screen buffer, home the cursor, and hide it
	fmt.Print("\033[?1049h\033[H\033[?25l")
	defer fmt.Print("\033[?1049l\033[?25h") // Restore main buffer and cursor on exit

	// Set terminal raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	events := make(chan Event)
	tracker := gather.NewLiveTracker()
	staticInfo := gather.GetSystemInfo(true) // Get basic static info quickly
	staticInfo.Packages = "Loading..."

	// Background resolver for packages count and slow metrics
	go func() {
		fullInfo := gather.GetSystemInfo(false)
		if fullInfo != nil {
			if fullInfo.Packages != "" && fullInfo.Packages != "N/A" {
				staticInfo.Packages = fullInfo.Packages
			}
		}
	}()

	// Input reader goroutine
	go func() {
		var buf [1]byte
		for {
			n, err := os.Stdin.Read(buf[:])
			if err != nil || n == 0 {
				return
			}
			events <- Event{Type: Key, Char: buf[0]}
		}
	}()

	// Ticker goroutine for live updates (refresh twice a second for high responsiveness!)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	go func() {
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

	// Trigger initial render
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
				fmt.Print("\033[H\033[2J") // Clear screen on tab switch
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
			// Clear screen to avoid residue characters during resize
			fmt.Print("\033[H\033[2J")
		}
	} else {
		if ld.termWidth == 0 {
			ld.termWidth = 80
			ld.termHeight = 24
		}
	}
}

func (ld *LiveDisplay) renderWarningScreen(theme Theme) {
	fmt.Print("\033[H")

	boxWidth := 46
	if ld.termWidth < boxWidth {
		boxWidth = ld.termWidth - 2
		if boxWidth < 10 {
			boxWidth = 10
		}
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  ⚠️   Terminal Size Too Constrained")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Current Size: %d x %d", ld.termWidth, ld.termHeight))
	lines = append(lines, "  Required Min: 50 x 15")
	lines = append(lines, "")
	lines = append(lines, "  Please resize your terminal window")
	lines = append(lines, "  or zoom out to restore the dashboard.")
	lines = append(lines, "")
	lines = append(lines, "  Press [Q] to quit.")

	paddingY := (ld.termHeight - len(lines) - 2) / 2
	if paddingY < 0 {
		paddingY = 0
	}

	for i := 0; i < paddingY; i++ {
		fmt.Print("\r\n")
	}

	box := drawBox("Warning", lines, boxWidth, theme)
	for _, l := range box {
		paddingX := (ld.termWidth - boxWidth) / 2
		if paddingX < 0 {
			paddingX = 0
		}
		fmt.Printf("%s%s\r\n", strings.Repeat(" ", paddingX), l)
	}

	totalHeightUsed := paddingY + len(box)
	for i := totalHeightUsed; i < ld.termHeight; i++ {
		fmt.Printf("%s\r\n", strings.Repeat(" ", ld.termWidth))
	}
}

func (ld *LiveDisplay) render() {
	distroKey := getDistroKey()
	theme := GetThemeForDistro(distroKey, false)

	// If terminal size is too small, show warning screen
	if ld.termWidth < 50 || ld.termHeight < 15 {
		ld.renderWarningScreen(theme)
		return
	}

	metrics, err := ld.tracker.GetMetrics()
	if err != nil {
		return
	}

	// Move cursor to home (top-left) to avoid flickering
	fmt.Print("\033[H")

	// Print dynamic tab header
	ld.printHeader(theme)

	switch ld.tab {
	case "processes":
		ld.renderProcesses(metrics, theme)
	case "network":
		ld.renderNetwork(metrics, theme)
	case "cores":
		ld.renderCores(metrics, theme)
	default:
		ld.renderDashboard(metrics, theme)
	}

	// Clear all leftover lines from this cursor position to the bottom of the screen
	fmt.Print("\033[J")

	// Print footer
	ld.printFooter(theme)
}

func (ld *LiveDisplay) printHeader(theme Theme) {
	w := ld.termWidth
	if w < 40 {
		w = 40
	}

	// Draw a beautiful, solid title bar at the top (inverse background of distro accent color)
	title := "  KERNELVIEW LIVE MONITOR  "
	spaces := w - len(title)
	if spaces < 0 {
		spaces = 0
	}
	fmt.Printf("%s%s%s%s%s\r\n", theme.Accent, "\033[1;7m", title, strings.Repeat(" ", spaces), theme.Reset)

	// Draw tabs on the next line with elegant spacing and pill/inverse highlighting
	tabD := "  1. DASHBOARD  "
	tabP := "  2. PROCESSES  "
	tabN := "  3. NETWORK    "
	tabC := "  4. CPU CORES  "

	if ld.tab == "dashboard" {
		tabD = fmt.Sprintf("%s\033[1;7m%s\033[0m", theme.Accent, tabD)
	} else {
		tabD = fmt.Sprintf("\033[90m%s\033[0m", tabD)
	}
	if ld.tab == "processes" {
		tabP = fmt.Sprintf("%s\033[1;7m%s\033[0m", theme.Accent, tabP)
	} else {
		tabP = fmt.Sprintf("\033[90m%s\033[0m", tabP)
	}
	if ld.tab == "network" {
		tabN = fmt.Sprintf("%s\033[1;7m%s\033[0m", theme.Accent, tabN)
	} else {
		tabN = fmt.Sprintf("\033[90m%s\033[0m", tabN)
	}
	if ld.tab == "cores" {
		tabC = fmt.Sprintf("%s\033[1;7m%s\033[0m", theme.Accent, tabC)
	} else {
		tabC = fmt.Sprintf("\033[90m%s\033[0m", tabC)
	}

	if ld.termHeight < 20 {
		fmt.Printf("  %s  %s  %s  %s\r\n", tabD, tabP, tabN, tabC)
	} else {
		fmt.Printf("\r\n  %s  %s  %s  %s\r\n\r\n", tabD, tabP, tabN, tabC)
	}
}

func (ld *LiveDisplay) printFooter(theme Theme) {
	w := ld.termWidth

	keyQ := fmt.Sprintf("%s\033[1;7m Q \033[0m", theme.Accent)
	key1 := fmt.Sprintf("%s\033[1;7m 1 \033[0m", theme.Accent)
	key2 := fmt.Sprintf("%s\033[1;7m 2 \033[0m", theme.Accent)
	key3 := fmt.Sprintf("%s\033[1;7m 3 \033[0m", theme.Accent)
	key4 := fmt.Sprintf("%s\033[1;7m 4 \033[0m", theme.Accent)

	footerText := fmt.Sprintf(" %s Quit   %s Dashboard   %s Processes   %s Network   %s Cores", keyQ, key1, key2, key3, key4)
	visualLen := utf8.RuneCountInString(stripAnsi(footerText))

	if w > visualLen+2 {
		padding := w - visualLen - 1
		fmt.Printf("\r\n%s%s", footerText, strings.Repeat(" ", padding))
	} else {
		fmt.Printf("\r\n%s", footerText)
	}
}

func (ld *LiveDisplay) renderDashboard(metrics *gather.LiveMetrics, theme Theme) {
	// Box widths: split termWidth in half, capped at 38 each side
	var boxWidth int
	minHeightNeeded := 28
	if metrics.GPUMetrics.HasGPU {
		minHeightNeeded = 33
	}
	isSingleCol := ld.termWidth < 80 || ld.termHeight < minHeightNeeded

	if isSingleCol {
		boxWidth = ld.termWidth - 2
		if boxWidth < 40 {
			boxWidth = 40
		}
	} else {
		boxWidth = (ld.termWidth - 3) / 2
		if boxWidth < 37 {
			boxWidth = 37
		}
		if boxWidth > 75 {
			boxWidth = 75
		}
	}

	// 1. Hardware Info Box (Right Column)
	hwLines := []string{
		formatBoxLine("CPU", ld.static.CPU, boxWidth-4, theme),
		formatBoxLine("GPU", ld.static.GPU, boxWidth-4, theme),
		formatBoxLine("Memory", ld.static.RAM, boxWidth-4, theme),
	}
	if ld.static.Swap != "" && ld.static.Swap != "None" && ld.static.Swap != "N/A" {
		hwLines = append(hwLines, formatBoxLine("Swap", ld.static.Swap, boxWidth-4, theme))
	} else {
		hwLines = append(hwLines, formatBoxLine("Swap", "None", boxWidth-4, theme))
	}
	hwLines = append(hwLines, formatBoxLine("Disk Size", ld.static.Disk, boxWidth-4, theme))

	// 2. System Info Box (Left Column)
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
		formatBoxLine("OS", ld.static.OS, boxWidth-4, theme),
		formatBoxLine("Host", ld.static.Host, boxWidth-4, theme),
		formatBoxLine("Kernel", ld.static.Kernel, boxWidth-4, theme),
		formatBoxLine("Uptime", metrics.Uptime, boxWidth-4, theme),
		formatBoxLine("WM/DE", wmDe, boxWidth-4, theme),
		formatBoxLine("Resolution", ld.static.Resolution, boxWidth-4, theme),
		formatBoxLine("Shell", ld.static.Shell, boxWidth-4, theme),
		formatBoxLine("Terminal", ld.static.Terminal, boxWidth-4, theme),
		formatBoxLine("Packages", ld.static.Packages, boxWidth-4, theme),
		formatBoxLine("Virt", ld.static.Virtualization, boxWidth-4, theme),
		formatBoxLine("Locale", ld.static.Locale, boxWidth-4, theme),
		formatBoxLine("Local IP", ld.static.IPAddress, boxWidth-4, theme),
	}

	// 3. Resource Stats Box (Right Column)
	resLines := []string{
		fmt.Sprintf("%sCPU Usage%s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.CPUUsage, boxWidth-15, theme)),
		fmt.Sprintf("%sRAM Usage%s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.RAMPercent, boxWidth-15, theme)),
		formatBoxLine("RAM Bytes", fmt.Sprintf("%s / %s", gather.FormatBytes(metrics.RAMUsed), gather.FormatBytes(metrics.RAMTotal)), boxWidth-4, theme),
	}
	if metrics.SwapTotal > 0 {
		resLines = append(resLines, fmt.Sprintf("%sSwap     %s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.SwapPercent, boxWidth-15, theme)))
	} else {
		resLines = append(resLines, formatBoxLine("Swap", "None", boxWidth-4, theme))
	}
	resLines = append(resLines, fmt.Sprintf("%sDisk (/) %s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.DiskPercent, boxWidth-15, theme)))

	cpuTempStr := "N/A"
	if metrics.Temperature > 0 {
		cpuTempStr = fmt.Sprintf("%.1f °C", metrics.Temperature)
	}
	resLines = append(resLines, formatBoxLine("CPU Temp", cpuTempStr, boxWidth-4, theme))

	// 4. GPU Telemetry Box (Right Column, Nvidia/AMD/Intel)
	var gpuLines []string
	if metrics.GPUMetrics.HasGPU {
		// Line 1: Usage
		gpuLines = append(gpuLines, fmt.Sprintf("%sGPU Usage%s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.GPUMetrics.GPUUsage, boxWidth-15, theme)))

		// Line 2: Memory Progress Bar
		if metrics.GPUMetrics.GPUMemUsage > 0 {
			gpuLines = append(gpuLines, fmt.Sprintf("%sGPU Mem  %s\033[90m: %s", theme.Key, theme.Reset, drawLiveProgressBar(metrics.GPUMetrics.GPUMemUsage, boxWidth-15, theme)))
		} else {
			gpuLines = append(gpuLines, formatBoxLine("GPU Freq Info", "Integrated Graphics", boxWidth-4, theme))
		}

		// Line 3: Memory / Freq stats
		if metrics.GPUMetrics.GPUMemUsage == 0 && metrics.GPUMetrics.GPUMemTotal > 0 {
			gpuLines = append(gpuLines, formatBoxLine("GPU Freq", fmt.Sprintf("%d MHz / %d MHz", metrics.GPUMetrics.GPUMemUsed, metrics.GPUMetrics.GPUMemTotal), boxWidth-4, theme))
		} else if metrics.GPUMetrics.GPUMemTotal > 0 {
			gpuLines = append(gpuLines, formatBoxLine("GPU Memory", fmt.Sprintf("%d MB / %d MB", metrics.GPUMetrics.GPUMemUsed, metrics.GPUMetrics.GPUMemTotal), boxWidth-4, theme))
		} else {
			gpuLines = append(gpuLines, formatBoxLine("GPU Memory", "N/A", boxWidth-4, theme))
		}

		// Line 4: Temperature
		gpuTempStr := "N/A"
		if metrics.GPUMetrics.GPUTemp > 0 {
			gpuTempStr = fmt.Sprintf("%.1f °C", metrics.GPUMetrics.GPUTemp)
		}
		gpuLines = append(gpuLines, formatBoxLine("GPU Temp", gpuTempStr, boxWidth-4, theme))
	}

	// 5. Network Box (Left Column)
	netLines := []string{
		formatBoxLine("Interface", metrics.NetIface, boxWidth-4, theme),
		formatBoxLine("Download", fmt.Sprintf("%s/s", gather.FormatBytes(uint64(metrics.NetRxSpeed))), boxWidth-4, theme),
		formatBoxLine("Upload", fmt.Sprintf("%s/s", gather.FormatBytes(uint64(metrics.NetTxSpeed))), boxWidth-4, theme),
		formatBoxLine("Total Rx", gather.FormatBytes(metrics.NetRxTotal), boxWidth-4, theme),
		formatBoxLine("Total Tx", gather.FormatBytes(metrics.NetTxTotal), boxWidth-4, theme),
	}

	// 6. Processes Box (Right Column)
	nameColWidth := 12
	if (boxWidth - 4) > 33 {
		nameColWidth += (boxWidth - 4) - 33
	}

	procHeaderLine := fmt.Sprintf("%s%-6s %-*s %6s %6s%s", theme.Key, "PID", nameColWidth, "NAME", "CPU%", "RAM", theme.Reset)
	procSepLine := "\033[90m" + strings.Repeat("─", boxWidth-4) + theme.Reset

	procLines := []string{
		procHeaderLine,
		procSepLine,
	}
	limit := 5
	actualProcCount := len(metrics.Processes)
	if actualProcCount > limit {
		actualProcCount = limit
	}
	for i := 0; i < actualProcCount; i++ {
		p := metrics.Processes[i]
		ramStr := formatShortBytes(p.RAM)

		cpuColor := theme.Value
		if p.CPU > 20.0 {
			cpuColor = "\033[1;31m"
		} else if p.CPU > 5.0 {
			cpuColor = "\033[1;33m"
		}

		line := fmt.Sprintf("%-6d %-*s %s%5.1f%% %s%6s", p.PID, nameColWidth, truncateString(p.Name, nameColWidth), cpuColor, p.CPU, theme.Value, ramStr)
		procLines = append(procLines, line)
	}
	// Pad with empty rows to keep the box height constant
	for i := actualProcCount; i < limit; i++ {
		procLines = append(procLines, strings.Repeat(" ", boxWidth-4))
	}

	// Dynamic layout rendering for constrained (single-column) screens
	if isSingleCol {
		var col []string

		// 1. Resource Monitor (always first)
		resBox := drawBox("Resource Monitor", resLines, boxWidth, theme)
		col = append(col, resBox...)
		currentHeight := len(col)
		availableHeight := ld.termHeight - 8 // Header & footer padding

		// 2. GPU Monitor
		if metrics.GPUMetrics.HasGPU && currentHeight+len(gpuLines)+2 <= availableHeight {
			gpuBox := drawBox("GPU Monitor", gpuLines, boxWidth, theme)
			col = append(col, gpuBox...)
			currentHeight = len(col)
		}

		// 3. Network Rates
		if currentHeight+len(netLines)+2 <= availableHeight {
			netBox := drawBox("Network Rates", netLines, boxWidth, theme)
			col = append(col, netBox...)
			currentHeight = len(col)
		}

		// 4. Top Processes
		remainingForProc := availableHeight - currentHeight - 4 // border (2) + header/sep (2)
		if remainingForProc >= 1 {
			procLimit := remainingForProc
			if procLimit > 5 {
				procLimit = 5
			}

			dynamicProcLines := []string{procHeaderLine, procSepLine}
			actualProcCount := len(metrics.Processes)
			if actualProcCount > procLimit {
				actualProcCount = procLimit
			}
			for i := 0; i < actualProcCount; i++ {
				p := metrics.Processes[i]
				ramStr := formatShortBytes(p.RAM)
				cpuColor := theme.Value
				if p.CPU > 20.0 {
					cpuColor = "\033[1;31m"
				} else if p.CPU > 5.0 {
					cpuColor = "\033[1;33m"
				}
				line := fmt.Sprintf("%-6d %-*s %s%5.1f%% %s%6s", p.PID, nameColWidth, truncateString(p.Name, nameColWidth), cpuColor, p.CPU, theme.Value, ramStr)
				dynamicProcLines = append(dynamicProcLines, line)
			}

			procBox := drawBox("Top Processes", dynamicProcLines, boxWidth, theme)
			col = append(col, procBox...)
			currentHeight = len(col)
		}

		// 5. Hardware Specs
		if currentHeight+len(hwLines)+2 <= availableHeight {
			hwBox := drawBox("Hardware Specs", hwLines, boxWidth, theme)
			col = append(col, hwBox...)
			currentHeight = len(col)
		}

		// 6. System Info
		if currentHeight+len(sysLines)+2 <= availableHeight {
			sysBox := drawBox("System Info", sysLines, boxWidth, theme)
			col = append(col, sysBox...)
		}

		for _, line := range col {
			fmt.Printf("%s\r\n", line)
		}
		return
	}

	// Pad either netLines or procLines so that left column and right column boxes align perfectly at the bottom!
	// Left column consists of System Info (sysLines) and Network Rates (netLines) (2 boxes = 4 lines border)
	// Right column consists of Hardware Specs (hwLines), Resource Monitor (resLines), GPU Monitor (gpuLines if Nvidia/AMD/Intel), and Top Processes (procLines)
	totalLeftLines := len(sysLines) + len(netLines) + 4
	totalRightLines := len(hwLines) + len(resLines) + len(procLines) + 6
	if metrics.GPUMetrics.HasGPU {
		totalRightLines += len(gpuLines) + 2
	}

	if totalLeftLines > totalRightLines {
		diff := totalLeftLines - totalRightLines
		for i := 0; i < diff; i++ {
			procLines = append(procLines, strings.Repeat(" ", boxWidth-4))
		}
	} else if totalRightLines > totalLeftLines {
		diff := totalRightLines - totalLeftLines
		for i := 0; i < diff; i++ {
			netLines = append(netLines, strings.Repeat(" ", boxWidth-4))
		}
	}

	sysBox := drawBox("System Info", sysLines, boxWidth, theme)
	netBox := drawBox("Network Rates", netLines, boxWidth, theme)

	hwBox := drawBox("Hardware Specs", hwLines, boxWidth, theme)
	resBox := drawBox("Resource Monitor", resLines, boxWidth, theme)

	var gpuBox []string
	if metrics.GPUMetrics.HasGPU {
		gpuBox = drawBox("GPU Monitor", gpuLines, boxWidth, theme)
	}

	procBox := drawBox("Top Processes", procLines, boxWidth, theme)

	// Display columns
	leftCol := append(sysBox, netBox...)

	var rightCol []string
	if metrics.GPUMetrics.HasGPU {
		rightCol = append(hwBox, append(resBox, append(gpuBox, procBox...)...)...)
	} else {
		rightCol = append(hwBox, append(resBox, procBox...)...)
	}

	printSideBySide(leftCol, rightCol, 2)
}

func (ld *LiveDisplay) renderProcesses(metrics *gather.LiveMetrics, theme Theme) {
	w := ld.termWidth
	if w > 100 {
		w = 100
	}

	pidWidth := 8
	cpuWidth := 8
	ramWidth := 10
	ramPercWidth := 8
	nameWidth := w - pidWidth - cpuWidth - ramWidth - ramPercWidth - 8
	if nameWidth < 8 {
		nameWidth = 8
	}

	totalWidth := pidWidth + nameWidth + cpuWidth + ramWidth + ramPercWidth + 4

	var procLines []string
	procLines = append(procLines, fmt.Sprintf("%s%-*s %-*s %*s %*s %*s%s",
		theme.Key, pidWidth, "PID", nameWidth, "NAME", cpuWidth, "CPU%", ramWidth, "RAM", ramPercWidth, "RAM%", theme.Reset))
	procLines = append(procLines, "\033[90m"+strings.Repeat("─", totalWidth)+theme.Reset)

	headerPadding := 8
	if ld.termHeight < 20 {
		headerPadding = 5
	}
	limit := ld.termHeight - headerPadding - 4 // content header (1) + sep (1) + borders (2)
	if limit < 1 {
		limit = 1
	}
	if len(metrics.Processes) < limit {
		limit = len(metrics.Processes)
	}

	for i := 0; i < limit; i++ {
		p := metrics.Processes[i]
		ramStr := gather.FormatBytes(p.RAM)

		cpuColor := theme.Value
		if p.CPU > 20.0 {
			cpuColor = "\033[1;31m"
		} else if p.CPU > 5.0 {
			cpuColor = "\033[1;33m"
		}

		ramColor := theme.Value
		if p.RAM > 500*1024*1024 {
			ramColor = "\033[1;31m"
		} else if p.RAM > 150*1024*1024 {
			ramColor = "\033[1;33m"
		}

		row := fmt.Sprintf("%-*d %-*s %s%*.1f%% %s%*s %s%*.1f%%",
			pidWidth, p.PID,
			nameWidth, truncateString(p.Name, nameWidth),
			cpuColor, cpuWidth-1, p.CPU,
			ramColor, ramWidth, ramStr,
			theme.Value, ramPercWidth-1, p.RAMPerc)
		procLines = append(procLines, row)
	}

	box := drawBox("Expanded Process Monitor", procLines, totalWidth+4, theme)
	for _, line := range box {
		fmt.Printf("%s\r\n", line)
	}
}

func (ld *LiveDisplay) renderNetwork(metrics *gather.LiveMetrics, theme Theme) {
	w := ld.termWidth
	if w < 50 {
		w = 50
	}
	if w > 85 {
		w = 85
	}

	var netLines []string

	// Hostname and static network config (only if height is abundant)
	if ld.termHeight >= 22 {
		netLines = append(netLines, formatBoxLine("Hostname", ld.static.Hostname, w-4, theme))
		netLines = append(netLines, formatBoxLine("Private IP", metrics.NetIface+" ("+ld.static.IPAddress+")", w-4, theme))
		if metrics.NetRxTotal > 0 {
			netLines = append(netLines, formatBoxLine("Total Download", gather.FormatBytes(metrics.NetRxTotal), w-4, theme))
			netLines = append(netLines, formatBoxLine("Total Upload", gather.FormatBytes(metrics.NetTxTotal), w-4, theme))
		}
		netLines = append(netLines, "")
	}

	colW := 15
	if w < 55 {
		colW = 12
	}
	netLines = append(netLines, fmt.Sprintf("%s%-*s %*s %*s%s", theme.Key, colW, "INTERFACE", colW, "RX SPEED", colW, "TX SPEED", theme.Reset))
	netLines = append(netLines, "\033[90m"+strings.Repeat("─", w-4)+theme.Reset)

	// List active interfaces with current speeds
	ifaces, err := net.Interfaces()
	if err == nil {
		headerPadding := 10
		if ld.termHeight < 22 {
			headerPadding = 5
		}
		maxIfaces := ld.termHeight - headerPadding - 6 // tabs/header/footer + table header/sep/borders
		if maxIfaces < 1 {
			maxIfaces = 1
		}

		count := 0
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 {
				continue
			}
			if count >= maxIfaces {
				netLines = append(netLines, "... (more interfaces truncated)")
				break
			}
			isLoopback := iface.Flags&net.FlagLoopback != 0
			name := iface.Name
			if isLoopback {
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

			netLines = append(netLines, fmt.Sprintf("%-*s %*s %*s", colW, name, colW, rxS, colW, txS))
			count++
		}
	}

	box := drawBox("Network Interfaces and Speed", netLines, w, theme)
	for _, line := range box {
		fmt.Printf("%s\r\n", line)
	}
}

func drawBox(title string, lines []string, width int, theme Theme) []string {
	var box []string

	borderColor := theme.Accent
	if borderColor == "" {
		borderColor = "\033[90m" // Fallback to dark gray
	}

	titlePart := " " + title + " "
	top := borderColor + "╭─" + theme.Reset + "\033[1m" + titlePart + theme.Reset + borderColor
	visualLen := utf8.RuneCountInString(stripAnsi(top))
	if visualLen < width-1 {
		top += strings.Repeat("─", width-visualLen-1) + "╮" + theme.Reset
	} else {
		top = top[:width-1] + "╮" + theme.Reset
	}
	box = append(box, top)

	for _, line := range lines {
		stripped := stripAnsi(line)
		lineVisualLen := utf8.RuneCountInString(stripped)
		var paddedLine string
		if lineVisualLen < width-4 {
			paddedLine = line + strings.Repeat(" ", width-4-lineVisualLen)
		} else {
			paddedLine = "\033[0m" + truncateString(stripped, width-4)
		}
		box = append(box, borderColor+"│ "+theme.Reset+paddedLine+borderColor+" │"+theme.Reset)
	}

	bottom := borderColor + "╰" + strings.Repeat("─", width-2) + "╯" + theme.Reset
	box = append(box, bottom)

	return box
}

func printSideBySide(left []string, right []string, spacing int) {
	max := len(left)
	if len(right) > max {
		max = len(right)
	}

	leftWidth := 0
	if len(left) > 0 {
		leftWidth = utf8.RuneCountInString(stripAnsi(left[0]))
	}

	rightWidth := 0
	if len(right) > 0 {
		rightWidth = utf8.RuneCountInString(stripAnsi(right[0]))
	}

	spacer := strings.Repeat(" ", spacing)

	for i := 0; i < max; i++ {
		var lPart string
		if i < len(left) {
			lPart = left[i]
		} else {
			lPart = strings.Repeat(" ", leftWidth)
		}

		var rPart string
		if i < len(right) {
			rPart = right[i]
		} else {
			rPart = strings.Repeat(" ", rightWidth)
		}

		fmt.Printf("%s%s%s\r\n", lPart, spacer, rPart)
	}
}

func formatBoxLine(key string, val string, width int, theme Theme) string {
	visualLen := utf8.RuneCountInString(key) + 2 + utf8.RuneCountInString(val)
	if visualLen < width {
		padding := strings.Repeat(" ", width-visualLen)
		return fmt.Sprintf("%s%s%s\033[90m: %s%s%s%s", theme.Key, key, theme.Reset, theme.Value, val, theme.Reset, padding)
	}
	allowedValLen := width - utf8.RuneCountInString(key) - 5
	if allowedValLen > 3 {
		val = truncateString(val, allowedValLen)
	} else {
		val = "..."
	}
	visualLen = utf8.RuneCountInString(key) + 2 + utf8.RuneCountInString(val)
	padding := ""
	if visualLen < width {
		padding = strings.Repeat(" ", width-visualLen)
	}
	return fmt.Sprintf("%s%s%s\033[90m: %s%s%s%s", theme.Key, key, theme.Reset, theme.Value, val, theme.Reset, padding)
}

func drawLiveProgressBar(percent float64, width int, theme Theme) string {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}

	barWidth := width - 7
	if barWidth < 5 {
		barWidth = 5
	}
	filled := int(float64(barWidth) * (percent / 100.0))
	empty := barWidth - filled

	color := "\033[1;32m" // Green
	if percent >= 85 {
		color = "\033[1;31m" // Red
	} else if percent >= 60 {
		color = "\033[1;33m" // Yellow
	}

	filledStr := color + strings.Repeat("█", filled) + "\033[0m"
	emptyStr := "\033[90m" + strings.Repeat("░", empty) + "\033[0m"

	return fmt.Sprintf("\033[90m[\033[0m%s%s\033[90m]\033[0m %3.0f%%", filledStr, emptyStr, percent)
}

func formatShortBytes(b uint64) string {
	ramMB := float64(b) / (1024 * 1024)
	if ramMB < 1.0 {
		ramKB := float64(b) / 1024
		return fmt.Sprintf("%.0fK", ramKB)
	}
	if ramMB < 1024 {
		return fmt.Sprintf("%.0fM", ramMB)
	}
	ramGB := ramMB / 1024
	return fmt.Sprintf("%.1fG", ramGB)
}

func (ld *LiveDisplay) renderCores(metrics *gather.LiveMetrics, theme Theme) {
	w := ld.termWidth
	if w > 90 {
		w = 90
	}

	var coreLines []string

	// Print some static CPU hardware info at the top (only if height is abundant)
	if ld.termHeight >= 20 {
		coreLines = append(coreLines, formatBoxLine("CPU Model", ld.static.CPU, w-4, theme))
		coreLines = append(coreLines, formatBoxLine("Cores/Threads", ld.static.CoresThreads, w-4, theme))
		if ld.static.CPUSpeed != "" {
			coreLines = append(coreLines, formatBoxLine("Base Speed", ld.static.CPUSpeed, w-4, theme))
		}
		coreLines = append(coreLines, "")
	}

	// 2-Column Grid for CPU Cores
	numCores := len(metrics.CPUCores)
	if numCores > 0 {
		boxContentWidth := w - 4
		colWidth := (boxContentWidth - 3) / 2
		if colWidth < 20 {
			colWidth = 20
		}

		headerPadding := 12
		if ld.termHeight < 20 {
			headerPadding = 7
		}
		maxCoreRows := ld.termHeight - headerPadding
		if maxCoreRows < 2 {
			maxCoreRows = 2
		}

		rowCount := 0
		for i := 0; i < numCores; i += 2 {
			if rowCount >= maxCoreRows {
				coreLines = append(coreLines, "... (more CPU cores truncated)")
				break
			}

			// Left CPU thread
			coreLeftVal := metrics.CPUCores[i]
			// Label is "CPU XX: " -> "CPU XX" is 6 chars. ": " is 2 chars. Total = 8.
			// Progress bar takes colWidth - 8.
			leftBar := drawLiveProgressBar(coreLeftVal, colWidth-8, theme)
			leftPart := fmt.Sprintf("%sCPU %-2d%s\033[90m: %s", theme.Key, i, theme.Reset, leftBar)

			rightPart := ""
			if i+1 < numCores {
				// Right CPU thread
				coreRightVal := metrics.CPUCores[i+1]
				rightBar := drawLiveProgressBar(coreRightVal, colWidth-8, theme)
				rightPart = fmt.Sprintf("%sCPU %-2d%s\033[90m: %s", theme.Key, i+1, theme.Reset, rightBar)
			} else {
				rightPart = strings.Repeat(" ", colWidth)
			}

			spacer := " \033[90m│\033[0m "
			row := fmt.Sprintf("%s%s%s", leftPart, spacer, rightPart)
			coreLines = append(coreLines, row)
			rowCount++
		}
	} else {
		coreLines = append(coreLines, "No per-core CPU telemetry available.")
	}

	box := drawBox("Per-CPU Telemetry", coreLines, w, theme)
	for _, line := range box {
		fmt.Printf("%s\r\n", line)
	}
}
