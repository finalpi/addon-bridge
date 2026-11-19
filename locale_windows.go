package main

import "syscall"

func systemLanguage() string {
	value, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	if value&0x3ff == 4 {
		return "zh-CN"
	}
	return "en"
}
