package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"KernelView-Go/display"
	"KernelView-Go/gather"
)

// version is set at build time
var version = "v1.0.0"

// handleJSONOutput handles all --json flag requests
func handleJSONOutput(processFlag, networkFlag, gpuFlag, fastFlag bool) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ") // Pretty-print JSON

	var data interface{}
	var err error

	if processFlag {
		data, err = gather.GetProcessList()
		if err != nil {
			log.Fatalf("Error getting process list: %v", err)
		}
	} else if networkFlag {
		data, err = gather.GetNetworkDetails()
		if err != nil {
			log.Printf("Warning: Encountered errors fetching network details: %v", err)
		}
	} else if gpuFlag {
		data = gather.GetGPUDetails()
	} else {
		data = gather.GetSystemInfo(fastFlag)
	}

	// Encode the data to stdout
	if err := encoder.Encode(data); err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
}

func main() {
	// Define flags
	var fastFlag bool
	flag.BoolVar(&fastFlag, "fast", false, "Run system info in fast mode: Skips slower checks.")
	flag.BoolVar(&fastFlag, "f", false, "Run system info in fast mode (shorthand).")

	var processFlag bool
	flag.BoolVar(&processFlag, "process", false, "Display list of running processes.")
	flag.BoolVar(&processFlag, "p", false, "Display list of running processes (shorthand).")

	var networkFlag bool
	flag.BoolVar(&networkFlag, "network", false, "Display detailed network information.")
	flag.BoolVar(&networkFlag, "n", false, "Display detailed network information (shorthand).")

	var gpuFlag bool
	flag.BoolVar(&gpuFlag, "gpu", false, "Display detailed GPU and graphics API report.")
	flag.BoolVar(&gpuFlag, "g", false, "Display detailed GPU and graphics API report (shorthand).")

	var liveFlag bool
	flag.BoolVar(&liveFlag, "live", false, "Start real-time live system monitoring dashboard.")
	flag.BoolVar(&liveFlag, "l", false, "Start real-time live system monitoring dashboard (shorthand).")

	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "Print the version and exit.")
	flag.BoolVar(&versionFlag, "v", false, "Print the version and exit (shorthand).")

	var jsonFlag bool
	flag.BoolVar(&jsonFlag, "json", false, "Output information as JSON.")

	var noColorFlag bool
	flag.BoolVar(&noColorFlag, "no-color", false, "Disable all color and formatting.")

	var mockFlag string
	flag.StringVar(&mockFlag, "mock", "", "Mock a system setup for testing (arch, ubuntu, macos, windows).")
	flag.StringVar(&mockFlag, "m", "", "Mock a system setup for testing (shorthand).")

	// Custom usage message
	flag.Usage = func() {
		earlyNoColor := false
		for _, arg := range os.Args {
			if arg == "--no-color" {
				earlyNoColor = true
				break
			}
		}
		if os.Getenv("NO_COLOR") != "" {
			earlyNoColor = true
		}
		display.PrintHelp(version, earlyNoColor)
	}

	flag.Parse()

	// Handle mock environment override
	if mockFlag != "" {
		display.MockDistro = strings.ToLower(mockFlag)
		gather.MockDistro = strings.ToLower(mockFlag)
	}

	// --- Handle Priority Flags ---

	// 1. Version flag
	if versionFlag {
		fmt.Println("KernelView Go " + version)
		os.Exit(0)
	}

	// 2. Real-time Live TUI Dashboard
	if liveFlag {
		display.StartLiveDashboard(fastFlag)
		os.Exit(0)
	}

	// 2. Check for mutually exclusive modes
	modeCount := 0
	if fastFlag {
		modeCount++
	}
	if processFlag {
		modeCount++
	}
	if networkFlag {
		modeCount++
	}
	if gpuFlag {
		modeCount++
	}

	if modeCount > 1 {
		fmt.Fprintf(os.Stderr, "Error: -f, -p, -n, and -g flags are mutually exclusive.\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// 3. Handle JSON output
	if jsonFlag {
		handleJSONOutput(processFlag, networkFlag, gpuFlag, fastFlag)
		os.Exit(0)
	}

	// --- Handle Standard Display ---

	var currentTheme display.Theme
	mode := "system" // Default mode

	if processFlag {
		mode = "process"
	} else if networkFlag {
		mode = "network"
	} else if gpuFlag {
		mode = "gpu"
	}

	// Select theme
	if noColorFlag {
		currentTheme = display.BlankTheme
	} else if fastFlag {
		currentTheme = display.FastTheme
	} else {
		currentTheme = display.NormalTheme
	}

	// --- Execute Selected Mode ---
	switch mode {
	case "process":
		processList, err := gather.GetProcessList()
		if err != nil {
			log.Fatalf("Error getting process list: %v", err)
		}
		display.DisplayProcessList(processList, currentTheme)
	case "network":
		networkInfo, err := gather.GetNetworkDetails()
		if err != nil {
			log.Printf("Warning: Encountered errors fetching some network details: %v", err)
		}
		if networkInfo != nil {
			display.DisplayNetworkInfo(networkInfo, currentTheme)
		} else {
			log.Fatalf("Error getting network details.")
		}
	case "gpu":
		gpuDetails := gather.GetGPUDetails()
		display.DisplayGPUInfo(gpuDetails, currentTheme)
	default: // "system" mode
		info := gather.GetSystemInfo(fastFlag) // Pass fastFlag here
		display.DisplaySystemInfo(info, currentTheme)
	}
}
