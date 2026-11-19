//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func systemLanguage() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				language := strings.Trim(line, " \t\r\n\"(),")
				if language != "" {
					return language
				}
			}
		}
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if value := os.Getenv(name); value != "" && value != "C" && value != "C.UTF-8" {
			return value
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return "en"
}
