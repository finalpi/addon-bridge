package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/gogpu/systray"
	"github.com/zalando/go-keyring"
)

//go:embed ui/*
var desktopFiles embed.FS

type credentials struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}
type desktopState struct {
	mu           sync.Mutex
	root         string
	token        string
	address      string
	busy         bool
	paused       bool
	startOnLogin bool
	lastSync     time.Time
	message      string
	failed       bool
	credentials  credentials
	jobs         chan []string
	stop         chan struct{}
	refreshTray  func()
}

func (d *desktopState) credentialService() string {
	hash := sha256.Sum256([]byte(d.root))
	return "AddonBridge-" + hex.EncodeToString(hash[:8])
}

func (d *desktopState) legacyCredentialService() string {
	hash := sha256.Sum256([]byte(d.root))
	return legacyCredentialPrefix + hex.EncodeToString(hash[:8])
}
func (d *desktopState) savedCredentials() (string, error) {
	value, err := keyring.Get(d.credentialService(), "gist")
	if err == keyring.ErrNotFound {
		return keyring.Get(d.legacyCredentialService(), "gist")
	}
	return value, err
}
func (d *desktopState) clearCredentials() error {
	for _, service := range []string{d.credentialService(), d.legacyCredentialService()} {
		if err := keyring.Delete(service, "gist"); err != nil && err != keyring.ErrNotFound {
			return err
		}
	}
	return nil
}
func (d *desktopState) child(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	d.mu.Lock()
	creds := d.credentials
	d.mu.Unlock()
	cmd.Env = append(os.Environ(), "ADDONBRIDGE_HOME="+d.root)
	if creds.Token != "" {
		cmd.Env = append(cmd.Env, "ADDONBRIDGE_GITHUB_TOKEN="+creds.Token)
	}
	if creds.Password != "" {
		cmd.Env = append(cmd.Env, "ADDONBRIDGE_PASSWORD="+creds.Password)
	}
	hideConsole(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}
func (d *desktopState) enqueue(args []string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.busy {
		return false
	}
	d.busy = true
	d.message = d.text("Working…", "正在处理…")
	d.failed = false
	d.jobs <- args
	return true
}
func (d *desktopState) worker() {
	for {
		select {
		case args := <-d.jobs:
			err := d.child(args...)
			if err == nil {
				switch args[0] {
				case "add", "edit", "remove", "settings", "browser", "profile":
					err = d.child("sync")
				}
			}
			d.mu.Lock()
			d.busy = false
			d.failed = err != nil
			if err != nil {
				d.message = err.Error()
			} else {
				d.message = d.text("Operation completed", "操作完成")
			}
			if args[0] == "sync" || args[0] == "start" {
				d.lastSync = time.Now()
				if err == nil {
					d.message = d.text("Synchronization completed", "同步已完成")
				}
			}
			d.mu.Unlock()
		case <-d.stop:
			return
		}
	}
}
func (d *desktopState) scheduler() {
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			d.mu.Lock()
			paused, last := d.paused, d.lastSync
			d.mu.Unlock()
			a, err := openApp(d.root)
			if err != nil || paused {
				continue
			}
			minutes := 60
			if r, ok := a.State.Records["interval"]; ok {
				json.Unmarshal(r.Value, &minutes)
			}
			if minutes < 5 || minutes > 10080 {
				minutes = 60
			}
			if time.Since(last) >= time.Duration(minutes)*time.Minute {
				d.enqueue([]string{"sync"})
			}
		case <-d.stop:
			return
		}
	}
}
func (d *desktopState) togglePause() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.paused = !d.paused
	if err := atomicJSON(filepath.Join(d.root, "desktop-settings.json"), map[string]bool{"paused": d.paused}); err != nil {
		d.message = err.Error()
		d.failed = true
	}
}
func (d *desktopState) setStartup(enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := setAutostart(enabled); err != nil {
		return err
	}
	d.startOnLogin = enabled
	return nil
}
func (d *desktopState) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/bridge/") {
		d.serveBridge(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	if r.Host != d.address {
		http.Error(w, "Invalid host", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+d.address {
		http.Error(w, "Invalid origin", 403)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		name := map[string]string{"/": "index.html", "/app.js": "app.js", "/style.css": "style.css", "/i18n.js": "i18n.js"}[r.URL.Path]
		if name == "" {
			http.NotFound(w, r)
			return
		}
		data, _ := desktopFiles.ReadFile("ui/" + name)
		content := map[string]string{"index.html": "text/html; charset=utf-8", "app.js": "application/javascript", "i18n.js": "application/javascript", "style.css": "text/css"}[name]
		w.Header().Set("Content-Type", content)
		w.Write(data)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+d.token {
		http.Error(w, "请从托盘重新打开管理面板", 401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/state" && r.Method == "GET" {
		d.mu.Lock()
		busy, paused, message, failed, last := d.busy, d.paused, d.message, d.failed, d.lastSync
		hasCredentials := d.credentials.Token != "" && d.credentials.Password != ""
		d.mu.Unlock()
		a, err := openApp(d.root)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		browsers := []map[string]any{}
		for _, name := range []string{"brave", "chrome", "edge", "chromium"} {
			_, err := browserExecutable(name)
			configured := false
			profiles, selected := discoverProfiles(userDataRoot(name))
			mode := "existing"
			for _, b := range a.Config.Browsers {
				if b.Name == name {
					configured = true
					selected = b.Profile
					mode = b.Mode
				}
			}
			browsers = append(browsers, map[string]any{"name": name, "available": err == nil, "configured": configured, "profiles": profiles, "profile": selected, "mode": mode})
		}
		interval := 60
		if record, ok := a.State.Records["interval"]; ok {
			json.Unmarshal(record.Value, &interval)
		}
		language, languagePreference := preferredLanguage(d.root)
		d.mu.Lock()
		startOnLogin := d.startOnLogin
		d.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"language": language, "languagePreference": languagePreference, "busy": busy, "paused": paused, "startOnLogin": startOnLogin, "message": message, "failed": failed, "lastSync": last, "credentials": hasCredentials, "gist": a.Config.Gist, "extensions": a.extensions(), "installed": a.State.Installed, "existing": a.State.Existing, "releases": a.State.Releases, "browsers": browsers, "interval": interval})
		return
	}
	if r.URL.Path != "/api/action" || r.Method != "POST" {
		http.Error(w, "Not found", 404)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var input struct {
		Action   string `json:"action"`
		Repo     string `json:"repo"`
		NewRepo  string `json:"newRepo"`
		Asset    string `json:"asset"`
		Browser  string `json:"browser"`
		Value    string `json:"value"`
		Token    string `json:"token"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
		Mode     string `json:"mode"`
		Profile  string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	var args []string
	switch input.Action {
	case "language":
		if input.Value != "auto" && input.Value != "en" && input.Value != "zh-CN" {
			http.Error(w, "Invalid language", 400)
			return
		}
		if err := atomicJSON(filepath.Join(d.root, "language.json"), map[string]string{"language": input.Value}); err != nil {
			http.Error(w, "Could not save language", 500)
			return
		}
		if d.refreshTray != nil {
			d.refreshTray()
		}
	case "copy-password":
		d.mu.Lock()
		secret := d.credentials.Password
		d.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"password": secret})
		return

	case "sync":
		args = []string{"sync"}
	case "pause":
		d.togglePause()
	case "startup":
		enabled := input.Value == "on" || input.Value == "true"
		if err := d.setStartup(enabled); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if d.refreshTray != nil {
			d.refreshTray()
		}
	case "restart-browser":
		args = []string{"restart-browser", input.Browser}
	case "add":
		args = []string{"add", input.Repo, input.Asset}
		if input.Asset == "" {
			args = args[:2]
		}
	case "remove":
		args = []string{"remove", input.Repo}
	case "edit":
		args = []string{"edit", input.Repo, input.NewRepo, input.Asset}
		if input.Asset == "" {
			args = args[:3]
		}
	case "check":
		args = []string{"check"}
	case "settings":
		args = []string{"settings", input.Repo, input.Value}
	case "browser":
		args = []string{"browser", "add", input.Browser}
	case "start":
		args = []string{"start", input.Browser}
	case "profile":
		args = []string{"profile", input.Browser, input.Mode, input.Profile}
	case "install-page":
		args = []string{"install-page", input.Browser}
	case "interval":
		args = []string{"interval", input.Value}
	case "gist":
		args = []string{"gist", input.Value}
	case "credentials":
		if input.Token == "" || len(input.Password) < 12 {
			http.Error(w, "请填写 Token，以及至少 12 个字符的密码", 400)
			return
		}
		d.mu.Lock()
		busy := d.busy
		d.mu.Unlock()
		if busy {
			http.Error(w, "请等待当前操作完成", 409)
			return
		}
		creds := credentials{input.Token, input.Password}
		if input.Remember {
			data, _ := json.Marshal(creds)
			if err := keyring.Set(d.credentialService(), "gist", string(data)); err != nil {
				http.Error(w, "系统凭据库保存失败，可取消记住后仅在本次使用", 500)
				return
			}
		}
		if !input.Remember {
			err := d.clearCredentials()
			if err != nil && err != keyring.ErrNotFound {
				http.Error(w, "无法删除之前保存的系统凭据", 500)
				return
			}
		}
		d.mu.Lock()
		d.credentials = creds
		d.message = "凭据已设置"
		d.failed = false
		d.mu.Unlock()
	default:
		http.Error(w, "Unknown action", 400)
		return
	}
	if len(args) > 0 && !d.enqueue(args) {
		http.Error(w, "正在处理上一项操作，请稍后再试", 409)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func openPanel(address string) {
	var cmd *exec.Cmd
	if exe, err := browserExecutable("chrome"); err == nil {
		cmd = exec.Command(exe, address)
	} else if exe, err := browserExecutable("edge"); err == nil {
		cmd = exec.Command(exe, address)
	} else if exe, err := browserExecutable("brave"); err == nil {
		cmd = exec.Command(exe, address)
	} else {
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address)
		case "darwin":
			cmd = exec.Command("open", address)
		default:
			cmd = exec.Command("xdg-open", address)
		}
	}
	cmd.Env = browserEnvironment()
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}
func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 2; y < 30; y++ {
		for x := 2; x < 30; x++ {
			img.Set(x, y, color.RGBA{29, 161, 137, 255})
		}
	}
	for y := 8; y < 25; y++ {
		for x := 9; x < 23; x++ {
			if x < 12 || x >= 20 || y < 11 || (y >= 15 && y < 18) {
				img.Set(x, y, color.White)
			}
		}
	}
	var out bytes.Buffer
	png.Encode(&out, img)
	return out.Bytes()
}
func desktop(root string, background bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(root, "desktop.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	endpointFile := filepath.Join(root, "desktop-endpoint.json")
	if !ok {
		var endpoint struct {
			URL string `json:"url"`
		}
		if err = readJSON(endpointFile, &endpoint); err != nil {
			return err
		}
		if !strings.HasPrefix(endpoint.URL, "http://127.0.0.1:") {
			return fmt.Errorf("invalid desktop endpoint")
		}
		if !background && appEnv("NO_OPEN") != "1" {
			openPanel(endpoint.URL)
		}
		return nil
	}
	defer lock.Unlock()
	d := &desktopState{root: root, token: randomID() + randomID(), jobs: make(chan []string, 1), stop: make(chan struct{}), message: "准备就绪"}
	if _, err = os.Stat(filepath.Join(root, "config.json")); os.IsNotExist(err) {
		if err = d.child("init"); err != nil {
			return err
		}
	}
	if data, err := d.savedCredentials(); err == nil {
		json.Unmarshal([]byte(data), &d.credentials)
	}
	if d.credentials.Token == "" {
		d.credentials = credentials{osToken(), password()}
	}
	var prefs struct {
		Paused bool `json:"paused"`
	}
	readJSON(filepath.Join(root, "desktop-settings.json"), &prefs)
	d.paused = prefs.Paused
	d.startOnLogin = autostartEnabled()
	listener, err := listenDesktop(root)
	if err != nil {
		return err
	}
	d.address = listener.Addr().String()
	panel := "http://" + d.address + "/#" + d.token
	if err = atomicJSON(endpointFile, map[string]string{"url": panel}); err != nil {
		listener.Close()
		return err
	}
	defer os.Remove(endpointFile)
	server := &http.Server{Handler: http.HandlerFunc(d.serve), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	defer server.Close()
	go d.worker()
	go d.scheduler()
	defer close(d.stop)
	tray := systray.New().SetIcon(trayIcon()).SetTooltip("AddonBridge · 扩展管理")
	menu := systray.NewMenu()
	openItem := menu.Add(d.text("Open dashboard", "打开管理面板"), func() { openPanel(panel) })
	syncItem := menu.Add(d.text("Sync now", "立即同步"), func() { d.enqueue([]string{"sync"}) })
	pauseItem := menu.Add(d.text("Pause / resume automatic sync", "暂停 / 恢复自动同步"), func() { d.togglePause() })
	var startupItem *systray.MenuItem
	startupItem = menu.AddCheckbox(d.text("Start with system", "随系统启动"), d.startOnLogin, func() {
		d.mu.Lock()
		enabled := !d.startOnLogin
		d.mu.Unlock()
		if err := d.setStartup(enabled); err != nil {
			d.mu.Lock()
			d.message = err.Error()
			d.failed = true
			d.mu.Unlock()
		} else {
			d.mu.Lock()
			checked := d.startOnLogin
			d.mu.Unlock()
			startupItem.SetChecked(checked)
		}
	})
	menu.AddSeparator()
	exitItem := menu.Add(d.text("Quit AddonBridge", "退出 AddonBridge"), func() {
		d.mu.Lock()
		busy := d.busy
		if !busy {
			d.busy = true
		}
		d.mu.Unlock()
		if busy {
			tray.ShowNotification("AddonBridge", "当前操作完成后再退出，避免中断同步。")
			return
		}
		tray.Remove()
	})
	d.refreshTray = func() {
		openItem.SetLabel(d.text("Open dashboard", "打开管理面板"))
		syncItem.SetLabel(d.text("Sync now", "立即同步"))
		pauseItem.SetLabel(d.text("Pause / resume automatic sync", "暂停 / 恢复自动同步"))
		startupItem.SetLabel(d.text("Start with system", "随系统启动"))
		d.mu.Lock()
		checked := d.startOnLogin
		d.mu.Unlock()
		startupItem.SetChecked(checked)
		exitItem.SetLabel(d.text("Quit AddonBridge", "退出 AddonBridge"))
	}
	go server.Serve(listener)
	tray.SetMenu(menu).OnClick(func() { openPanel(panel) }).Show()
	if !background && appEnv("NO_OPEN") != "1" {
		openPanel(panel)
	}
	if !d.paused {
		d.enqueue([]string{"sync"})
	}
	return tray.Run()
}
