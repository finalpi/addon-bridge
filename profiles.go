package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type UserProfile struct {
	Directory string `json:"directory"`
	Name      string `json:"name"`
}

func userDataRoot(browser string) string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), filepath.FromSlash(map[string]string{"chrome": "Google/Chrome/User Data", "brave": "BraveSoftware/Brave-Browser/User Data", "edge": "Microsoft/Edge/User Data", "chromium": "Chromium/User Data"}[browser]))
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", filepath.FromSlash(map[string]string{"chrome": "Google/Chrome", "brave": "BraveSoftware/Brave-Browser", "edge": "Microsoft Edge", "chromium": "Chromium"}[browser]))
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, filepath.FromSlash(map[string]string{"chrome": "google-chrome", "brave": "BraveSoftware/Brave-Browser", "edge": "microsoft-edge", "chromium": "chromium"}[browser]))
	}
}
func discoverProfiles(root string) ([]UserProfile, string) {
	var local struct {
		Profile struct {
			LastUsed string `json:"last_used"`
			Cache    map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	readJSON(filepath.Join(root, "Local State"), &local)
	result := []UserProfile{}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		dir := entry.Name()
		if !entry.IsDir() || (dir != "Default" && !strings.HasPrefix(dir, "Profile ")) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, dir, "Preferences")); err != nil {
			continue
		}
		name := local.Profile.Cache[dir].Name
		if name == "" {
			name = dir
		}
		result = append(result, UserProfile{dir, name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Directory < result[j].Directory })
	selected := "Default"
	for _, p := range result {
		if p.Directory == local.Profile.LastUsed {
			return result, p.Directory
		}
	}
	if len(result) > 0 {
		selected = result[0].Directory
	}
	return result, selected
}
func defaultProfile(browser string) string {
	_, selected := discoverProfiles(userDataRoot(browser))
	return selected
}
func validProfile(browser, profile string) bool {
	profiles, _ := discoverProfiles(userDataRoot(browser))
	for _, p := range profiles {
		if p.Directory == profile {
			return true
		}
	}
	return false
}
func (a *App) openExisting(b Browser, page string) error {
	if !validProfile(b.Name, b.Profile) {
		return fmt.Errorf("所选资料不存在，请先正常打开浏览器建立资料，再在界面选择")
	}
	cmd := exec.Command(b.Executable, "--profile-directory="+b.Profile, page)
	cmd.Env = browserEnvironment()
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func (a *App) selectProfile(name, mode, profile string) error {
	if mode != "existing" && mode != "managed" {
		return fmt.Errorf("invalid profile mode")
	}
	if mode == "existing" && !validProfile(name, profile) {
		return fmt.Errorf("profile not found")
	}
	for i := range a.Config.Browsers {
		if a.Config.Browsers[i].Name == name {
			a.Config.Browsers[i].Mode = mode
			a.Config.Browsers[i].Profile = profile
			return a.saveConfig()
		}
	}
	return fmt.Errorf("请先添加浏览器")
}
