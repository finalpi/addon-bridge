//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
)

func setAutostart(enabled bool) error {
	home, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "autostart", "addonbridge.desktop")
	if !enabled {
		err = os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("[Desktop Entry]\nType=Application\nName=AddonBridge\nExec="+strconv.Quote(exe)+" gui --background\nX-GNOME-Autostart-enabled=true\nNoDisplay=true\n"), 0600)
}
func autostartEnabled() bool {
	home, _ := os.UserConfigDir()
	_, err := os.Stat(filepath.Join(home, "autostart", "addonbridge.desktop"))
	return err == nil
}
