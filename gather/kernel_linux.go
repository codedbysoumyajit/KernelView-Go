//go:build linux

package gather

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/host"
)

var (
	reCompilerShort  = regexp.MustCompile(`(?i)(gcc|clang)\s*(?:\([^\)]+\)\s*)?(\d+\.\d+(?:\.\d+)?)`)
	reLockdownChoice = regexp.MustCompile(`\[([a-zA-Z0-9_-]+)\]`)
)

type modDepInfo struct {
	name     string
	depCount int
}

// GetKernelDetails gathers essential Linux kernel telemetry with concurrent subsystem collection.
func GetKernelDetails() *KernelDetails {
	details := &KernelDetails{
		Architecture: runtime.GOARCH,
	}

	// Phase 1: Core release & command line (needed by dependent gatherers)
	var wg1 sync.WaitGroup
	wg1.Add(2)
	go func() {
		defer wg1.Done()
		gatherKernelVersion(details)
	}()
	go func() {
		defer wg1.Done()
		gatherKernelCmdline(details)
	}()
	wg1.Wait()

	// Phase 2: Concurrent gathering of all independent subsystems
	var wg2 sync.WaitGroup
	wg2.Add(11)

	go func() {
		defer wg2.Done()
		gatherKernelModules(details)
	}()
	go func() {
		defer wg2.Done()
		gatherKernelTaint(details)
	}()
	go func() {
		defer wg2.Done()
		gatherSecureBoot(details)
	}()
	go func() {
		defer wg2.Done()
		gatherKernelLockdown(details)
	}()
	go func() {
		defer wg2.Done()
		gatherSecurityLSM(details)
	}()
	go func() {
		defer wg2.Done()
		gatherKASLR(details)
	}()
	go func() {
		defer wg2.Done()
		gatherEBPF(details)
	}()
	go func() {
		defer wg2.Done()
		gatherCPUScaling(details)
	}()
	go func() {
		defer wg2.Done()
		gatherClockSource(details)
	}()
	go func() {
		defer wg2.Done()
		gatherVulnerabilities(details)
	}()
	go func() {
		defer wg2.Done()
		gatherKernelUpdates(details)
	}()

	wg2.Wait()

	return details
}

// gatherKernelVersion reads and parses /proc/version with minimal overhead
func gatherKernelVersion(kd *KernelDetails) {
	procVerBytes, err := os.ReadFile("/proc/version")
	if err == nil {
		raw := strings.TrimSpace(string(procVerBytes))

		parts := strings.Fields(raw)
		if len(parts) >= 3 && parts[0] == "Linux" && parts[1] == "version" {
			kd.Release = parts[2]
			if strings.HasSuffix(kd.Release, ".x86_64") || runtime.GOARCH == "amd64" {
				kd.Architecture = "x86_64"
			} else if strings.HasSuffix(kd.Release, ".aarch64") || runtime.GOARCH == "arm64" {
				kd.Architecture = "aarch64"
			}
		}

		if match := reCompilerShort.FindStringSubmatch(raw); len(match) > 2 {
			kd.Compiler = fmt.Sprintf("%s %s", strings.ToLower(match[1]), match[2])
		}

		if strings.Contains(raw, "PREEMPT_DYNAMIC") {
			kd.Preemption = "PREEMPT_DYNAMIC"
		} else if strings.Contains(raw, "PREEMPT_RT") {
			kd.Preemption = "PREEMPT_RT"
		} else if strings.Contains(raw, "PREEMPT") {
			kd.Preemption = "PREEMPT"
		} else if strings.Contains(raw, "SMP") {
			kd.Preemption = "SMP"
		}

		hashIdx := strings.Index(raw, "#")
		if hashIdx != -1 {
			verAndDate := raw[hashIdx:]
			dateParts := strings.Fields(verAndDate)
			for i, p := range dateParts {
				if p == "Sun" || p == "Mon" || p == "Tue" || p == "Wed" || p == "Thu" || p == "Fri" || p == "Sat" {
					if i+3 < len(dateParts) {
						kd.BuildDate = fmt.Sprintf("%s %s %s", dateParts[i+1], dateParts[i+2], dateParts[len(dateParts)-1])
					} else {
						kd.BuildDate = strings.Join(dateParts[i:], " ")
					}
					break
				}
			}
		}
	}

	// Quick proc fallback before slower host.Info
	if kd.Release == "" {
		if rel, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			kd.Release = strings.TrimSpace(string(rel))
		}
	}

	// Last-resort fallback to gopsutil host.Info
	if kd.Release == "" {
		if h, err := host.Info(); err == nil {
			kd.Release = h.KernelVersion
			if h.KernelArch != "" {
				kd.Architecture = h.KernelArch
			}
		}
	}
}

// gatherKernelCmdline reads /proc/cmdline and creates a human-friendly cleaned version
func gatherKernelCmdline(kd *KernelDetails) {
	cmdlineBytes, err := os.ReadFile("/proc/cmdline")
	if err == nil {
		raw := strings.TrimSpace(string(cmdlineBytes))
		kd.CommandLine = raw
		kd.CleanCommandLine = formatCleanCmdline(raw)
	} else {
		kd.CommandLine = "N/A"
		kd.CleanCommandLine = "N/A"
	}
}

// formatCleanCmdline filters out bulky bootloader noise (BOOT_IMAGE) and shortens long UUIDs
func formatCleanCmdline(raw string) string {
	if raw == "" || raw == "N/A" {
		return raw
	}

	tokens := strings.Fields(raw)
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if strings.HasPrefix(t, "BOOT_IMAGE=") {
			continue
		}
		if strings.HasPrefix(t, "root=UUID=") {
			uuid := strings.TrimPrefix(t, "root=UUID=")
			if len(uuid) > 8 {
				cleaned = append(cleaned, fmt.Sprintf("root=UUID(%s…)", uuid[:8]))
			} else {
				cleaned = append(cleaned, t)
			}
		} else if strings.HasPrefix(t, "resume=UUID=") {
			uuid := strings.TrimPrefix(t, "resume=UUID=")
			if len(uuid) > 8 {
				cleaned = append(cleaned, fmt.Sprintf("resume=UUID(%s…)", uuid[:8]))
			} else {
				cleaned = append(cleaned, t)
			}
		} else {
			cleaned = append(cleaned, t)
		}
	}

	if len(cleaned) == 0 {
		return raw
	}
	return strings.Join(cleaned, " ")
}

// gatherKernelModules parses /proc/modules and counts available modules from modules.dep
func gatherKernelModules(kd *KernelDetails) {
	modulesFile, err := os.Open("/proc/modules")
	if err != nil {
		return
	}
	defer modulesFile.Close()

	var totalBytes uint64
	var loadedCount int
	depList := make([]modDepInfo, 0, 32)

	scanner := bufio.NewScanner(modulesFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		modName := fields[0]
		sizeBytes, _ := strconv.ParseUint(fields[1], 10, 64)
		loadedCount++
		totalBytes += sizeBytes

		if len(fields) >= 4 && fields[3] != "-" {
			depsRaw := strings.TrimSuffix(fields[3], ",")
			depItems := strings.Split(depsRaw, ",")
			cnt := 0
			for _, d := range depItems {
				if d != "" && d != "-" {
					cnt++
				}
			}
			if cnt > 0 {
				depList = append(depList, modDepInfo{name: modName, depCount: cnt})
			}
		}
	}

	kd.LoadedModulesCount = loadedCount
	kd.LoadedModulesMemory = formatBytes(totalBytes)

	sort.Slice(depList, func(i, j int) bool {
		return depList[i].depCount > depList[j].depCount
	})
	topDepParts := make([]string, 0, 4)
	limit := 4
	if len(depList) < limit {
		limit = len(depList)
	}
	for i := 0; i < limit; i++ {
		topDepParts = append(topDepParts, fmt.Sprintf("%s (%d)", depList[i].name, depList[i].depCount))
	}
	if len(topDepParts) > 0 {
		kd.TopDependencies = strings.Join(topDepParts, ", ")
	}

	// Optimized modules.dep counting: read entire file once and use bytes.Count (assembly accelerated)
	if kd.Release != "" {
		depPath := filepath.Join("/lib/modules", kd.Release, "modules.dep")
		if depBytes, err := os.ReadFile(depPath); err == nil {
			count := bytes.Count(depBytes, []byte("\n"))
			kd.TotalAvailableModules = count
			if count >= kd.LoadedModulesCount {
				kd.UnloadedModulesCount = count - kd.LoadedModulesCount
			}
		}
	}
}

// gatherKernelTaint reads and decodes the taint bitmask from /proc/sys/kernel/tainted
func gatherKernelTaint(kd *KernelDetails) {
	data, err := os.ReadFile("/proc/sys/kernel/tainted")
	if err != nil {
		kd.TaintStatus = "Unknown"
		return
	}

	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		kd.TaintStatus = "Unknown"
		return
	}

	if val == 0 {
		kd.TaintStatus = "0 (Clean)"
		return
	}

	taintDefinitions := [...]struct {
		bit  int
		flag string
	}{
		{0, "Proprietary module"},
		{1, "Forced module"},
		{2, "Non-SMP CPU"},
		{3, "Forced unload"},
		{4, "Machine check"},
		{5, "Bad page"},
		{6, "User taint"},
		{7, "Kernel died"},
		{8, "ACPI override"},
		{9, "Warning issued"},
		{10, "Staging driver"},
		{11, "Firmware workaround"},
		{12, "Out-of-tree module"},
		{13, "Unsigned module"},
		{14, "Soft lockup"},
		{15, "Livepatched"},
		{16, "Auxiliary"},
		{17, "Struct rand"},
		{18, "Test module"},
	}

	var activeFlags []string
	for _, td := range taintDefinitions {
		if (val & (1 << td.bit)) != 0 {
			activeFlags = append(activeFlags, td.flag)
		}
	}

	if len(activeFlags) > 0 {
		kd.TaintStatus = fmt.Sprintf("%d (%s)", val, strings.Join(activeFlags, ", "))
	} else {
		kd.TaintStatus = fmt.Sprintf("%d (Tainted)", val)
	}
}

// gatherSecureBoot checks UEFI Secure Boot status without slow external commands unless necessary
func gatherSecureBoot(kd *KernelDetails) {
	// 1. Direct sysfs efivar read: fastest and zero-overhead
	matches, _ := filepath.Glob("/sys/firmware/efi/efivars/SecureBoot-*")
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err == nil && len(b) >= 5 {
			if b[4] == 1 {
				kd.SecureBoot = "Enabled (UEFI)"
				return
			} else if b[4] == 0 {
				kd.SecureBoot = "Disabled (UEFI)"
				return
			}
		}
	}

	// 2. Check if EFI runtime exists at all
	if _, err := os.Stat("/sys/firmware/efi"); err != nil {
		kd.SecureBoot = "Not Supported (BIOS)"
		return
	}

	// 3. Fallback to mokutil only if EFI is present but efivars is unreadable
	if out, err := exec.Command("mokutil", "--sb-state").Output(); err == nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "secureboot enabled") {
			kd.SecureBoot = "Enabled (UEFI)"
			return
		} else if strings.Contains(s, "secureboot disabled") {
			kd.SecureBoot = "Disabled (UEFI)"
			return
		}
	}

	kd.SecureBoot = "Disabled (UEFI)"
}

// gatherKernelLockdown reads /sys/kernel/security/lockdown
func gatherKernelLockdown(kd *KernelDetails) {
	data, err := os.ReadFile("/sys/kernel/security/lockdown")
	if err == nil {
		s := strings.TrimSpace(string(data))
		if match := reLockdownChoice.FindStringSubmatch(s); len(match) > 1 {
			mode := strings.ToLower(match[1])
			switch mode {
			case "none":
				kd.Lockdown = "Disabled"
			case "integrity":
				kd.Lockdown = "Integrity"
			case "confidentiality":
				kd.Lockdown = "Confidentiality"
			default:
				kd.Lockdown = mode
			}
			return
		}
	}
	kd.Lockdown = "None"
}

// gatherSecurityLSM checks active primary LSM (SELinux / AppArmor)
func gatherSecurityLSM(kd *KernelDetails) {
	if enforceData, err := os.ReadFile("/sys/fs/selinux/enforce"); err == nil {
		val := strings.TrimSpace(string(enforceData))
		if val == "1" {
			kd.SecurityLSM = "SELinux (Enforcing)"
			return
		} else if val == "0" {
			kd.SecurityLSM = "SELinux (Permissive)"
			return
		}
	}

	if aaData, err := os.ReadFile("/sys/module/apparmor/parameters/enabled"); err == nil {
		if strings.TrimSpace(string(aaData)) == "Y" {
			kd.SecurityLSM = "AppArmor (Enabled)"
			return
		}
	}

	if data, err := os.ReadFile("/sys/kernel/security/lsm"); err == nil {
		raw := strings.TrimSpace(string(data))
		if raw != "" {
			kd.SecurityLSM = "Standard"
			return
		}
	}
	kd.SecurityLSM = "None"
}

// gatherKASLR checks Kernel Address Space Layout Randomization
func gatherKASLR(kd *KernelDetails) {
	if strings.Contains(kd.CommandLine, "nokaslr") {
		kd.KASLR = "Disabled (nokaslr)"
		return
	}
	data, err := os.ReadFile("/proc/sys/kernel/randomize_va_space")
	if err == nil {
		val := strings.TrimSpace(string(data))
		switch val {
		case "2":
			kd.KASLR = "Enabled (Full)"
		case "1":
			kd.KASLR = "Enabled (Conservative)"
		case "0":
			kd.KASLR = "Disabled"
		default:
			kd.KASLR = "Enabled"
		}
	} else {
		kd.KASLR = "Enabled"
	}
}

// gatherEBPF checks eBPF JIT compiler status and security hardening
func gatherEBPF(kd *KernelDetails) {
	jitData, err := os.ReadFile("/proc/sys/net/core/bpf_jit_enable")
	if err != nil {
		return
	}
	jitVal := strings.TrimSpace(string(jitData))
	if jitVal == "1" || jitVal == "2" {
		unprivData, err := os.ReadFile("/proc/sys/kernel/unprivileged_bpf_disabled")
		if err == nil {
			unprivVal := strings.TrimSpace(string(unprivData))
			if unprivVal == "1" || unprivVal == "2" {
				kd.EBPFJIT = "Enabled (Hardened)"
				return
			}
		}
		kd.EBPFJIT = "Enabled"
	} else if jitVal == "0" {
		kd.EBPFJIT = "Disabled (Interpreter)"
	}
}

// gatherCPUScaling reads CPU scaling driver and governor
func gatherCPUScaling(kd *KernelDetails) {
	driverBytes, _ := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_driver")
	govBytes, _ := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor")
	driver := strings.TrimSpace(string(driverBytes))
	gov := strings.TrimSpace(string(govBytes))

	if driver != "" && gov != "" {
		kd.CPUScaling = fmt.Sprintf("%s (%s)", driver, gov)
	} else if gov != "" {
		kd.CPUScaling = gov
	} else if driver != "" {
		kd.CPUScaling = driver
	}
}

// gatherClockSource reads current clocksource and memory page size
func gatherClockSource(kd *KernelDetails) {
	clockBytes, _ := os.ReadFile("/sys/devices/system/clocksource/clocksource0/current_clocksource")
	clock := strings.TrimSpace(string(clockBytes))
	pageSize := os.Getpagesize()
	pageStr := fmt.Sprintf("%d KB page", pageSize/1024)

	if clock != "" {
		kd.ClockSource = fmt.Sprintf("%s (%s)", clock, pageStr)
	} else {
		kd.ClockSource = pageStr
	}
}

// gatherVulnerabilities inspects /sys/devices/system/cpu/vulnerabilities/*
func gatherVulnerabilities(kd *KernelDetails) {
	dir := "/sys/devices/system/cpu/vulnerabilities"
	entries, err := os.ReadDir(dir)
	if err != nil {
		kd.Mitigations = "N/A"
		return
	}

	countMitigated := 0
	countNotAffected := 0
	countVulnerable := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(string(contentBytes)))
		if strings.HasPrefix(status, "mitigation:") || strings.Contains(status, "mitigation") {
			countMitigated++
		} else if strings.Contains(status, "not affected") {
			countNotAffected++
		} else if strings.Contains(status, "vulnerable") {
			countVulnerable++
		}
	}

	kd.Mitigations = fmt.Sprintf("%d active, %d not affected, %d vulnerable",
		countMitigated, countNotAffected, countVulnerable)
}

// gatherKernelUpdates compares running kernel release with installed kernels in /boot
func gatherKernelUpdates(kd *KernelDetails) {
	vmlinuzFiles, _ := filepath.Glob("/boot/vmlinuz-*")
	var installed []string
	for _, f := range vmlinuzFiles {
		base := filepath.Base(f)
		ver := strings.TrimPrefix(base, "vmlinuz-")
		if !strings.Contains(ver, "rescue") {
			installed = append(installed, ver)
		}
	}

	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		kd.UpdateStatus = "Reboot required (update installed)"
		return
	}

	// Single-pass maximum find (O(N) instead of O(N log N) sort)
	if len(installed) > 0 && kd.Release != "" {
		newest := installed[0]
		for _, ver := range installed[1:] {
			if compareKernelVersions(ver, newest) > 0 {
				newest = ver
			}
		}
		if newest != kd.Release && compareKernelVersions(newest, kd.Release) > 0 {
			kd.UpdateStatus = fmt.Sprintf("Reboot required (Installed: %s)", newest)
			return
		}
	}

	kd.UpdateStatus = "Up to date"
}

// compareKernelVersions compares two kernel release strings semantically
func compareKernelVersions(v1, v2 string) int {
	clean1 := strings.Split(v1, "-")[0]
	clean2 := strings.Split(v2, "-")[0]
	p1 := strings.Split(clean1, ".")
	p2 := strings.Split(clean2, ".")

	maxLen := len(p1)
	if len(p2) > maxLen {
		maxLen = len(p2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(p1) {
			n1, _ = strconv.Atoi(p1[i])
		}
		if i < len(p2) {
			n2, _ = strconv.Atoi(p2[i])
		}
		if n1 != n2 {
			return n1 - n2
		}
	}
	return strings.Compare(v1, v2)
}

// formatBytes helper
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
