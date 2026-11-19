package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func setAutostart(enabled bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue("AddonBridge")
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue("AddonBridge", windows.ComposeCommandLine([]string{exe, "gui", "--background"}))
}

func autostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	command, _, err := key.GetStringValue("AddonBridge")
	return err == nil && command != ""
}
