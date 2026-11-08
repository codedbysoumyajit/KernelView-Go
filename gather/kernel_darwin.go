//go:build darwin

package gather

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

// GetKernelDetails gathers essential macOS XNU kernel telemetry.
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

	if out, err := exec.Command("csrutil", "status").Output(); err == nil {
		s := string(out)
		if strings.Contains(s, "enabled") {
			details.Lockdown = "SIP Enabled"
		} else if strings.Contains(s, "disabled") {
			details.Lockdown = "SIP Disabled"
		}
	}

	if out, err := exec.Command("nvram", "boot-args").Output(); err == nil {
		args := strings.TrimSpace(string(out))
		if strings.HasPrefix(args, "boot-args") {
			details.CommandLine = strings.TrimSpace(strings.TrimPrefix(args, "boot-args"))
		}
	}

	details.SecureBoot = "Enabled (Secure Enclave)"
	details.TaintStatus = "Clean (Apple KEXT Signing)"
	details.SecurityLSM = "Apple Sandbox / AMFI"
	details.UpdateStatus = "Managed by macOS Update"

	return details
}
