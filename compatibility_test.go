package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentCompatibility(t *testing.T) {
	t.Setenv("EXTSYNC_PASSWORD", "previous")
	t.Setenv("ADDONBRIDGE_PASSWORD", "")
	if password() != "previous" {
		t.Fatal("previous environment alias is not accepted")
	}
	t.Setenv("ADDONBRIDGE_PASSWORD", "current")
	if password() != "current" {
		t.Fatal("current environment name must take precedence")
	}
}
func TestDataDirectoryCompatibility(t *testing.T) {
	t.Setenv("EXTSYNC_HOME", t.TempDir())
	t.Setenv("ADDONBRIDGE_HOME", "")
	old, err := dataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if old != os.Getenv("EXTSYNC_HOME") {
		t.Fatal("existing custom root changed")
	}
	current := filepath.Join(t.TempDir(), "custom")
	t.Setenv("ADDONBRIDGE_HOME", current)
	root, err := dataRoot()
	if err != nil || root != current {
		t.Fatal("new custom root not selected")
	}
}
