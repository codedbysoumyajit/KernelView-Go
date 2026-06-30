//go:build windows

package main

import (
	"os"
	"syscall"
)

func init() {
	// Enable Virtual Terminal Processing for Windows Console (ANSI Escape Sequences)
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err == nil {
		mode |= 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
		syscall.SetConsoleMode(handle, mode)
	}

	// Also enable virtual terminal processing on Stderr in case warnings/errors are printed
	handleStderr := syscall.Handle(os.Stderr.Fd())
	var modeStderr uint32
	if err := syscall.GetConsoleMode(handleStderr, &modeStderr); err == nil {
		modeStderr |= 0x0004
		syscall.SetConsoleMode(handleStderr, modeStderr)
	}
}
