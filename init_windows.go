//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	// Enable Virtual Terminal Processing for Windows Console (ANSI Escape Sequences)
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err == nil {
		mode |= 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
		windows.SetConsoleMode(handle, mode)
	}

	// Also enable virtual terminal processing on Stderr in case warnings/errors are printed
	handleStderr := windows.Handle(os.Stderr.Fd())
	var modeStderr uint32
	if err := windows.GetConsoleMode(handleStderr, &modeStderr); err == nil {
		modeStderr |= 0x0004
		windows.SetConsoleMode(handleStderr, modeStderr)
	}
}
