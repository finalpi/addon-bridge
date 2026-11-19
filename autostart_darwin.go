//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
)

func setAutostart(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "local.addonbridge.plist")
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
	data := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><plist version=\"1.0\"><dict><key>Label</key><string>local.addonbridge</string><key>ProgramArguments</key><array><string>" + xmlEscape(exe) + "</string><string>gui</string><string>--background</string></array><key>RunAtLoad</key><true/></dict></plist>")
	return os.WriteFile(path, data, 0600)
}
func autostartEnabled() bool {
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", "local.addonbridge.plist"))
	return err == nil
}
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}
