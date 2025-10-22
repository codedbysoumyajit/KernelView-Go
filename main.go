package main

import (
	"flag"
	"fmt"
	"os"

	// Import local packages using the module path defined in go.mod
	"KernelView-Go/display"
	"KernelView-Go/gather"
)

func main() {
	// Define flags with shortcuts and detailed usage messages
	var fastFlag bool
	flag.BoolVar(&fastFlag, "fast", false, "Run in fast mode: Skips slower checks like CPU usage, packages, languages, temperature, network speed, and open ports for quicker results.")
	flag.BoolVar(&fastFlag, "f", false, "Run in fast mode (shorthand).")

	// Custom usage message for --help / -h
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nDescription:\n")
		fmt.Fprintf(os.Stderr, "  KernelView Go displays system information.\n")
		fmt.Fprintf(os.Stderr, "  Default mode performs a comprehensive scan (slower).\n")
		fmt.Fprintf(os.Stderr, "  Fast mode (-f, --fast) provides essential info instantly by skipping slower checks.\n")
	}

	flag.Parse()

	// Select theme based on flag
	var currentTheme display.Theme
	if fastFlag {
		currentTheme = display.FastTheme // Use exported theme
	} else {
		currentTheme = display.NormalTheme // Use exported theme
	}

	// Call the gather package's function
	info := gather.GetSystemInfo(fastFlag)

	// Call the display package's function
	display.DisplaySystemInfo(info, currentTheme)
}
