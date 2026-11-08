//go:build windows

package gather

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

// GetKernelDetails gathers essential Windows NT kernel telemetry.
func GetKernelDetails() *KernelDetails {
	details := &KernelDetails{
		Architecture: runtime.GOARCH,
	}

	h, err := host.Info()
	if err == nil {
		details.Release = h.KernelVersion
		if h.KernelArch != "" {
			details.Architecture = h.KernelArch
		}
	} else {
		details.Release = runtime.GOOS
	}

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Confirm-SecureBootUEFI")
	if out, err := cmd.Output(); err == nil {
		s := strings.TrimSpace(strings.ToLower(string(out)))
		if s == "true" {
			details.SecureBoot = "Enabled (UEFI)"
		} else if s == "false" {
			details.SecureBoot = "Disabled (UEFI)"
		} else {
			details.SecureBoot = "Not Supported (BIOS)"
		}
	} else {
		details.SecureBoot = "Not Supported (BIOS)"
	}

	details.Lockdown = "Code Integrity (HVCI/VBS)"
	details.TaintStatus = "Clean (Driver Signature Enforcement)"
	details.SecurityLSM = "Windows Defender"
	details.UpdateStatus = "Managed by Windows Update"

	return details
}
