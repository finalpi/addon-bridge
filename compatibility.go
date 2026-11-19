package main

import (
	"os"
	"path/filepath"
)

// Persisted identifiers are versioned independently of the product name.
// Keeping these values stable preserves installed extension IDs and encrypted data.
const (
	syncDomain             = "extensionsync/v1"
	syncFilePrefix         = "extensionsync-"
	extensionKeyDomain     = "extensionsync/v1/"
	bridgePage             = "extensionsync-bridge.html"
	bridgeWorker           = "extensionsync-worker.js"
	legacyDataDirectory    = "ExtensionSync"
	legacyCredentialPrefix = "ExtensionSync-"
)

func appEnv(name string) string {
	if value := os.Getenv("ADDONBRIDGE_" + name); value != "" {
		return value
	}
	return os.Getenv("EXTSYNC_" + name)
}

func dataRoot() (string, error) {
	if root := appEnv("HOME"); root != "" {
		return filepath.Abs(root)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	current := filepath.Join(base, "AddonBridge")
	if _, err = os.Stat(filepath.Join(current, "config.json")); err == nil {
		return current, nil
	}
	previous := filepath.Join(base, legacyDataDirectory)
	if _, err = os.Stat(filepath.Join(previous, "config.json")); err == nil {
		return previous, nil
	}
	return current, nil
}
