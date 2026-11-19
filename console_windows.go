package main

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

func showDesktopError(err error) {
	message, _ := syscall.UTF16PtrFromString(fmt.Sprint(err))
	title, _ := syscall.UTF16PtrFromString("AddonBridge")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}

func hideConsole(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
