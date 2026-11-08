package gather

// KernelDetails holds essential, high-value information about the kernel, modules, security parameters, and mitigations.
type KernelDetails struct {
	Release               string `json:"release"`
	Architecture          string `json:"architecture"`
	Compiler              string `json:"compiler,omitempty"`
	BuildDate             string `json:"build_date,omitempty"`
	Preemption            string `json:"preemption,omitempty"`
	CommandLine           string `json:"command_line"`
	CleanCommandLine      string `json:"clean_command_line,omitempty"`
	LoadedModulesCount    int    `json:"loaded_modules_count"`
	LoadedModulesMemory   string `json:"loaded_modules_memory"`
	UnloadedModulesCount  int    `json:"unloaded_modules_count"`
	TotalAvailableModules int    `json:"total_available_modules"`
	TopDependencies       string `json:"top_dependencies,omitempty"`
	TaintStatus           string `json:"taint_status"`
	SecureBoot            string `json:"secure_boot"`
	Lockdown              string `json:"lockdown"`
	SecurityLSM           string `json:"security_lsm,omitempty"`
	KASLR                 string `json:"kaslr,omitempty"`
	EBPFJIT               string `json:"ebpf_jit,omitempty"`
	CPUScaling            string `json:"cpu_scaling,omitempty"`
	ClockSource           string `json:"clock_source,omitempty"`
	Mitigations           string `json:"mitigations"`
	UpdateStatus          string `json:"update_status"`
}
