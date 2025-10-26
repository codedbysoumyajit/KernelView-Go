package main

import (
	"flag"
	"fmt"
	"log" // Import log package for error handling
	"os"

	// Import local packages using the module path defined in go.mod
	"KernelView-Go/display"
	"KernelView-Go/gather"
)

func main() {
	// Define flags
	var fastFlag bool
	flag.BoolVar(&fastFlag, "fast", false, "Run in fast mode: Skips slower checks.")
	flag.BoolVar(&fastFlag, "f", false, "Run in fast mode (shorthand).")

	var processFlag bool // New flag for process list
	flag.BoolVar(&processFlag, "process", false, "Display list of running processes.")
	flag.BoolVar(&processFlag, "p", false, "Display list of running processes (shorthand).")


	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nDescription:\n")
		fmt.Fprintf(os.Stderr, "  KernelView Go displays system information.\n")
		fmt.Fprintf(os.Stderr, "  Default mode performs a comprehensive scan (slower).\n")
		fmt.Fprintf(os.Stderr, "  Fast mode (-f, --fast) provides essential info instantly.\n")
		fmt.Fprintf(os.Stderr, "  Process mode (-p, --process) shows a list of running processes.\n") // Added description
	}

	flag.Parse()

	// Select theme (use normal theme for process list for now)
	var currentTheme display.Theme
	if fastFlag && !processFlag { // Only use fast theme if -f is set AND -p is NOT
		currentTheme = display.FastTheme
	} else {
		currentTheme = display.NormalTheme
	}


	// --- Conditional Execution ---
	if processFlag {
		// Get and display process list
		processList, err := gather.GetProcessList()
		if err != nil {
			log.Fatalf("Error getting process list: %v", err) // Use log.Fatalf for critical errors
		}
		display.DisplayProcessList(processList, currentTheme)
	} else {
		// Get and display system info (existing logic)
		info := gather.GetSystemInfo(fastFlag)
		display.DisplaySystemInfo(info, currentTheme)
	}
}
