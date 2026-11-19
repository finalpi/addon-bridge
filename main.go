package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

func password() string { return appEnv("PASSWORD") }
func osToken() string  { return appEnv("GITHUB_TOKEN") }
func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) == 1 || (len(os.Args) > 1 && os.Args[1] == "gui") {
			showDesktopError(err)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	root, err := dataRoot()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "gui" {
		background := len(args) == 2 && args[1] == "--background"
		return desktop(root, background)
	}
	if args[0] == "help" {
		fmt.Print(help)
		return nil
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(root, "process.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("another AddonBridge process is active; stop run before changing configuration")
	}
	defer lock.Unlock()
	if args[0] == "init" {
		if _, err = os.Stat(filepath.Join(root, "config.json")); err == nil {
			return fmt.Errorf("already initialized at %s", root)
		}
		a := &App{Root: root, Config: Config{Device: randomID()}, State: State{Schema: 1, Records: map[string]Record{}, Installed: map[string]Installed{}, Baselines: map[string]map[string]json.RawMessage{}, Releases: map[string]ReleaseStatus{}}}
		a.put("interval", 60, false)
		if err = a.saveConfig(); err != nil {
			return err
		}
		if err = a.save(); err != nil {
			return err
		}
		fmt.Println("Initialized", root)
		return nil
	}
	a, err := openApp(root)
	if err != nil {
		return fmt.Errorf("open configuration (run init first): %w", err)
	}
	switch args[0] {
	case "bootstrap-sync":
		return a.bootstrapSync()
	case "install-existing":
		return a.cycle()
	case "add":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: add OWNER/REPO [ASSET_GLOB]")
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(args[1], "https://github.com/"), "/")
		if !repoPattern.MatchString(repo) {
			return fmt.Errorf("invalid GitHub repository")
		}
		asset := "*chrome*.zip"
		if len(args) == 3 {
			asset = args[2]
		}
		e := Extension{Repo: repo, Asset: asset}
		if old, ok := a.extensions()[repo]; ok {
			e.Settings = old.Settings
		}
		a.put("ext/"+repo, e, false)
		if err = a.save(); err != nil {
			return err
		}
		fmt.Println("Added", repo)
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: remove OWNER/REPO")
		}
		if _, ok := a.extensions()[args[1]]; !ok {
			return fmt.Errorf("extension not configured")
		}
		a.put("ext/"+args[1], nil, true)
		return a.save()
	case "edit":
		if len(args) < 3 || len(args) > 4 {
			return fmt.Errorf("usage: edit OWNER/REPO NEW_OWNER/REPO [ASSET_GLOB]")
		}
		oldRepo, newRepo := args[1], args[2]
		old, ok := a.extensions()[oldRepo]
		if !ok {
			return fmt.Errorf("extension not configured")
		}
		if !repoPattern.MatchString(newRepo) {
			return fmt.Errorf("invalid GitHub repository")
		}
		asset := old.Asset
		if len(args) == 4 {
			asset = args[3]
		}
		if asset == "" || asset == "*.zip" {
			asset = "*chrome*.zip"
		}
		updated := Extension{Repo: newRepo, Asset: asset, Settings: old.Settings}
		a.put("ext/"+oldRepo, nil, true)
		a.put("ext/"+newRepo, updated, false)
		return a.save()
	case "settings":
		if len(args) != 3 || (args[2] != "on" && args[2] != "off") {
			return fmt.Errorf("usage: settings OWNER/REPO on|off")
		}
		e, ok := a.extensions()[args[1]]
		if !ok {
			return fmt.Errorf("extension not configured")
		}
		e.Settings = args[2] == "on"
		a.put("ext/"+e.Repo, e, false)
		if !e.Settings {
			for key := range a.State.Records {
				if strings.HasPrefix(key, "setting/"+e.Repo+"/") {
					a.put(key, nil, true)
				}
			}
			for key := range a.State.Baselines {
				if strings.Contains(key, "/"+e.Repo+"/") {
					delete(a.State.Baselines, key)
				}
			}
		}
		return a.save()
	case "interval":
		if len(args) != 2 {
			return fmt.Errorf("usage: interval MINUTES")
		}
		minutes, err := strconv.Atoi(args[1])
		if err != nil || minutes < 5 || minutes > 10080 {
			return fmt.Errorf("interval must be 5..10080 minutes")
		}
		a.put("interval", minutes, false)
		return a.save()
	case "browser":
		if len(args) < 3 || args[1] != "add" || len(args) > 4 {
			return fmt.Errorf("usage: browser add chrome|brave|edge|chromium [EXECUTABLE]")
		}
		name := args[2]
		if name != "chrome" && name != "brave" && name != "edge" && name != "chromium" {
			return fmt.Errorf("unknown browser")
		}
		var exe string
		if len(args) == 4 {
			exe, err = filepath.Abs(args[3])
		} else {
			exe, err = browserExecutable(name)
		}
		if err != nil {
			return err
		}
		if _, err = os.Stat(exe); err != nil {
			return err
		}
		for i, b := range a.Config.Browsers {
			if b.Name == name {
				a.Config.Browsers[i].Executable = exe
				return a.saveConfig()
			}
		}
		a.Config.Browsers = append(a.Config.Browsers, Browser{Name: name, Executable: exe, Mode: "existing", Profile: defaultProfile(name)})
		return a.saveConfig()
	case "profile":
		if len(args) != 4 {
			return fmt.Errorf("usage: profile BROWSER existing|managed PROFILE_DIRECTORY")
		}
		return a.selectProfile(args[1], args[2], args[3])
	case "install-page":
		if len(args) != 2 {
			return fmt.Errorf("usage: install-page BROWSER")
		}
		for _, b := range a.Config.Browsers {
			if b.Name == args[1] {
				return a.openExisting(b, "chrome://extensions/")
			}
		}
		return fmt.Errorf("browser not configured")
	case "gist":
		if len(args) != 2 {
			return fmt.Errorf("usage: gist create|GIST_ID")
		}
		if osToken() == "" || len(password()) < 12 {
			return fmt.Errorf("set ADDONBRIDGE_GITHUB_TOKEN and ADDONBRIDGE_PASSWORD (12+ characters)")
		}
		if args[1] == "create" {
			if a.Config.Gist != "" {
				return fmt.Errorf("a Gist is already configured")
			}
			content, err := seal(Snapshot{1, a.State.Records}, password())
			if err != nil {
				return err
			}
			body := map[string]any{"description": "AddonBridge encrypted configuration", "public": false, "files": map[string]any{syncFilePrefix + a.Config.Device + ".json": map[string]string{"content": content}}}
			data, err := request("POST", githubAPI+"/gists", body, true)
			if err != nil {
				return err
			}
			var result struct {
				ID string `json:"id"`
			}
			if err = json.Unmarshal(data, &result); err != nil {
				return err
			}
			a.Config.Gist = result.ID
		} else {
			if !regexp.MustCompile(`^[a-fA-F0-9]{5,64}$`).MatchString(args[1]) {
				return fmt.Errorf("invalid Gist ID")
			}
			a.Config.Gist = args[1]
			if err = a.pull(); err != nil {
				return err
			}
			if err = a.save(); err != nil {
				return err
			}
		}
		if err = a.saveConfig(); err != nil {
			return err
		}
		fmt.Println("Gist", a.Config.Gist)
		return nil
	case "status":
		fmt.Println("Home:", a.Root, "\nGist:", a.Config.Gist)
		repos := []string{}
		for repo := range a.extensions() {
			repos = append(repos, repo)
		}
		sort.Strings(repos)
		for _, repo := range repos {
			fmt.Printf("%s settings=%v\n", repo, a.extensions()[repo].Settings)
		}
		for _, b := range a.Config.Browsers {
			c, err := a.connect(b)
			state := "stopped"
			if err == nil {
				state = "connected"
				c.socket.Close()
			}
			fmt.Println(b.Name, state)
		}
		return nil
	case "start":
		if len(args) != 2 {
			return fmt.Errorf("usage: start BROWSER")
		}
		for _, b := range a.Config.Browsers {
			if b.Name == args[1] {
				if b.Mode != "managed" {
					if err = a.openExisting(b, "about:blank"); err != nil {
						return err
					}
					return a.cycle()
				}
				c, err := a.launch(b, false)
				if err != nil {
					return err
				}
				c.socket.Close()
				return a.cycle()
			}
		}
		return fmt.Errorf("browser not configured")
	case "restart-browser":
		if len(args) != 2 {
			return fmt.Errorf("usage: restart-browser BROWSER")
		}
		for _, b := range a.Config.Browsers {
			if b.Name == args[1] {
				if b.Mode == "managed" {
					return fmt.Errorf("managed profile restart is controlled by AddonBridge")
				}
				return a.openExisting(b, browserRestartURL(b.Name))
			}
		}
		return fmt.Errorf("browser not configured")
	case "sync", "update":
		return a.cycle()
	case "check":
		var checkErrors []error
		for _, e := range a.extensions() {
			if err := a.checkLatest(e); err != nil {
				checkErrors = append(checkErrors, err)
			}
		}
		if err := a.save(); err != nil {
			return err
		}
		return errors.Join(checkErrors...)
	case "run":
		if len(args) > 2 {
			return fmt.Errorf("usage: run [BROWSER]")
		}
		if len(args) == 2 {
			found := false
			for _, b := range a.Config.Browsers {
				if b.Name == args[1] {
					if b.Mode != "managed" {
						return fmt.Errorf("现有资料模式请运行 gui，由托盘提供连接服务")
					}
					c, err := a.launch(b, false)
					if err != nil {
						return err
					}
					c.socket.Close()
					found = true
				}
			}
			if !found {
				return fmt.Errorf("browser not configured")
			}
		}
		stopped := make(chan os.Signal, 1)
		signal.Notify(stopped, os.Interrupt)
		defer signal.Stop(stopped)
		for {
			if err = a.cycle(); err != nil {
				fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), err)
			}
			minutes := 60
			if record, ok := a.State.Records["interval"]; ok {
				json.Unmarshal(record.Value, &minutes)
			}
			if minutes < 5 || minutes > 10080 {
				minutes = 60
			}
			timer := time.NewTimer(time.Duration(minutes) * time.Minute)
			select {
			case <-stopped:
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	default:
		return fmt.Errorf("unknown command %q; use help", args[0])
	}
}

func (a *App) cycle() error {
	clients := map[string]*CDP{}
	present := map[string]map[string]bool{}
	for _, b := range a.Config.Browsers {
		if b.Mode != "managed" {
			continue
		}
		if c, err := a.connect(b); err == nil {
			clients[b.Name] = c
			defer c.socket.Close()
			var list struct {
				Extensions []struct {
					ID      string `json:"id"`
					Enabled bool   `json:"enabled"`
				} `json:"extensions"`
			}
			if err = c.call("Extensions.getExtensions", map[string]any{}, &list); err != nil {
				return fmt.Errorf("%s extension API unavailable: %w", b.Name, err)
			}
			present[b.Name] = map[string]bool{}
			for _, e := range list.Extensions {
				present[b.Name][e.ID] = e.Enabled
			}
		}
	}
	for _, b := range a.Config.Browsers {
		if c := clients[b.Name]; c != nil {
			for repo, e := range a.extensions() {
				if in, ok := a.State.Installed[b.Name+"/"+repo]; ok && present[b.Name][in.ID] {
					if err := a.capture(c, b, e, in); err != nil {
						return fmt.Errorf("capture %s %s: %w", b.Name, repo, err)
					}
				}
			}
		}
	}
	// Persist newly captured changes even if the network is offline.
	if err := a.save(); err != nil {
		return err
	}
	if err := a.pull(); err != nil {
		return err
	}
	if err := a.save(); err != nil {
		return err
	}
	var failures []error
	for key, entry := range a.State.Existing {
		if record, ok := a.State.Records["ext/"+entry.Repo]; ok && record.Deleted {
			entry.Status = "removed"
			a.State.Existing[key] = entry
			continue
		}
		selected := false
		for _, b := range a.Config.Browsers {
			if b.Mode != "managed" && b.Name == entry.Browser && b.Profile == entry.Profile {
				selected = true
			}
		}
		if !selected {
			entry.Status = "inactive"
			a.State.Existing[key] = entry
		}
	}
	for repo, e := range a.extensions() {
		dir, revision, err := preparePackage(a, e)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, b := range a.Config.Browsers {
			if b.Mode != "managed" {
				if err := a.prepareExisting(b, e, dir, revision); err != nil {
					failures = append(failures, err)
				}
				continue
			}
			c := clients[b.Name]
			if c == nil {
				continue
			}
			key := b.Name + "/" + repo
			old := a.State.Installed[key]
			// Loading the same directory also restores extensions removed outside the manager.
			if old.Revision != revision || old.ID == "" || !present[b.Name][old.ID] {
				var loaded struct {
					ID string `json:"id"`
				}
				if err = c.call("Extensions.loadUnpacked", map[string]string{"path": dir}, &loaded); err != nil {
					failures = append(failures, fmt.Errorf("install %s in %s: %w", repo, b.Name, err))
					continue
				}
				a.State.Installed[key] = Installed{revision, dir, loaded.ID}
				fmt.Println("Installed", repo, "in", b.Name, revision)
			}
			if err = a.restore(c, b, e, a.State.Installed[key]); err != nil {
				failures = append(failures, fmt.Errorf("restore %s: %w", repo, err))
			}
		}
	}
	for key, in := range a.State.Installed {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			continue
		}
		r, ok := a.State.Records["ext/"+parts[1]]
		if !ok || !r.Deleted {
			continue
		}
		if c := clients[parts[0]]; c != nil {
			if !present[parts[0]][in.ID] {
				delete(a.State.Installed, key)
				continue
			}
			if err := c.call("Extensions.uninstall", map[string]string{"id": in.ID}, nil); err != nil {
				failures = append(failures, err)
			} else {
				delete(a.State.Installed, key)
			}
		}
	}
	if err := a.save(); err != nil {
		return err
	}
	if err := a.push(); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

const help = `AddonBridge — GitHub extension manager

  gui                          Open management panel and system tray (default)
  init                         Create local configuration
  browser add NAME [PATH]       Chrome, Brave, Edge or Chromium
  profile NAME MODE DIRECTORY  Select existing profile (default) or managed mode
  install-page NAME            Open extensions page in the selected existing profile
  add OWNER/REPO [ASSET_GLOB]    Track the latest stable GitHub release ZIP
  remove OWNER/REPO             Remove from managed browsers on next sync
  settings OWNER/REPO on|off    Opt into local/sync extension storage sync
  interval MINUTES              Set automatic update interval (default 60)
  gist create|GIST_ID           Create or join an encrypted secret Gist
  start BROWSER                Open managed browser and synchronize once
  restart-browser BROWSER      Request the browser's built-in restart
  sync                         Sync settings, packages and connected browsers
  update                       Same as sync
  run [BROWSER]                Optionally open browser, then sync until Ctrl+C
  status                       Show configuration and browser connectivity

Environment:
  ADDONBRIDGE_HOME                  Data directory (default OS config/AddonBridge)
  ADDONBRIDGE_GITHUB_TOKEN          GitHub token with Gist permission
  ADDONBRIDGE_PASSWORD              Shared encryption password, 12+ characters

Browser profiles persist under ADDONBRIDGE_HOME/profiles. Start each browser using
this program. run keeps a sleeping low-overhead process for automatic updates.
`
