//go:build !linux && !windows && !darwin

package gather

import (
	"runtime"

	"github.com/shirou/gopsutil/v3/host"
)

// GetKernelDetails gathers essential fallback kernel telemetry for BSD and other operating systems.
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

	details.Lockdown = "N/A"
	details.TaintStatus = "Clean"
	details.SecureBoot = "N/A"
	details.SecurityLSM = "N/A"
	details.UpdateStatus = "N/A"

	return details
}
