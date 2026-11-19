package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type CDP struct {
	socket *websocket.Conn
	seq    int
}

func browserEnvironment() []string {
	result := []string{}
	for _, entry := range os.Environ() {
		name := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		switch name {
		case "ADDONBRIDGE_GITHUB_TOKEN", "ADDONBRIDGE_PASSWORD", "EXTSYNC_GITHUB_TOKEN", "EXTSYNC_PASSWORD", "GH_TOKEN", "GITHUB_TOKEN":
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (c *CDP) call(method string, params any, result any) error {
	return c.sessionCall("", method, params, result)
}
func (c *CDP) sessionCall(session, method string, params any, result any) error {
	c.seq++
	id := c.seq
	message := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		message["sessionId"] = session
	}
	c.socket.SetWriteDeadline(time.Now().Add(30 * time.Second))
	c.socket.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err := c.socket.WriteJSON(message); err != nil {
		return err
	}
	for {
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := c.socket.ReadJSON(&response); err != nil {
			return err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("%s: %s", method, response.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(response.Result, result)
		}
		return nil
	}
}
func browserExecutable(name string) (string, error) {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		locations := map[string]string{"chrome": "Google/Chrome/Application/chrome.exe", "brave": "BraveSoftware/Brave-Browser/Application/brave.exe", "edge": "Microsoft/Edge/Application/msedge.exe", "chromium": "Chromium/Application/chrome.exe"}
		for _, base := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if rel, ok := locations[name]; ok {
				candidates = append(candidates, filepath.Join(base, filepath.FromSlash(rel)))
			}
		}
	case "darwin":
		locations := map[string]string{"chrome": "Google Chrome.app/Contents/MacOS/Google Chrome", "brave": "Brave Browser.app/Contents/MacOS/Brave Browser", "edge": "Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "chromium": "Chromium.app/Contents/MacOS/Chromium"}
		if rel, ok := locations[name]; ok {
			candidates = append(candidates, filepath.Join("/Applications", rel))
		}
	default:
		names := map[string][]string{"chrome": {"google-chrome", "google-chrome-stable"}, "brave": {"brave-browser", "brave-browser-stable"}, "edge": {"microsoft-edge", "microsoft-edge-stable"}, "chromium": {"chromium", "chromium-browser"}}
		for _, n := range names[name] {
			if p, err := exec.LookPath(n); err == nil {
				return p, nil
			}
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s executable not found; use browser add NAME ABSOLUTE_PATH", name)
}

func browserRestartURL(name string) string {
	if name == "brave" {
		return "brave://restart"
	}
	if name == "edge" {
		return "edge://restart"
	}
	return "chrome://restart"
}
func (a *App) connect(b Browser) (*CDP, error) {
	profile := filepath.Join(a.Root, "profiles", b.Name)
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		return nil, fmt.Errorf("invalid debugging endpoint")
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid debugging port")
	}
	endpoint := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(endpoint, "/devtools/browser/") {
		return nil, fmt.Errorf("invalid browser endpoint")
	}
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second, NetDial: func(network, address string) (net.Conn, error) {
		return net.DialTimeout(network, address, 2*time.Second)
	}}
	socket, _, err := dialer.Dial("ws://127.0.0.1:"+strconv.Itoa(port)+endpoint, nil)
	if err != nil {
		return nil, err
	}
	return &CDP{socket: socket}, nil
}
func (a *App) launch(b Browser, headless bool) (*CDP, error) {
	if c, err := a.connect(b); err == nil {
		return c, nil
	}
	profile := filepath.Join(a.Root, "profiles", b.Name)
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, err
	}
	// Stale endpoints belong only to this application's private profile.
	os.Remove(filepath.Join(profile, "DevToolsActivePort"))
	args := []string{"--user-data-dir=" + profile, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1", "--enable-unsafe-extension-debugging", "--no-first-run", "--no-default-browser-check"}
	if headless {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")
	cmd := exec.Command(b.Executable, args...)
	cmd.Env = browserEnvironment()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go cmd.Wait()
	for i := 0; i < 80; i++ {
		time.Sleep(250 * time.Millisecond)
		if c, err := a.connect(b); err == nil {
			if err = c.call("Extensions.getExtensions", map[string]any{}, nil); err == nil {
				return c, nil
			}
			c.socket.Close()
		}
	}
	return nil, fmt.Errorf("%s did not expose its private debugging endpoint", b.Name)
}
func (c *CDP) storageSession(id string) (string, func(), error) {
	var target struct {
		ID string `json:"targetId"`
	}
	err := c.call("Target.createTarget", map[string]any{"url": "chrome-extension://" + id + "/" + bridgePage, "background": true}, &target)
	if err != nil {
		return "", nil, err
	}
	closeTarget := func() { c.call("Target.closeTarget", map[string]string{"targetId": target.ID}, nil) }
	var attached struct {
		ID string `json:"sessionId"`
	}
	err = c.call("Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, &attached)
	if err != nil {
		closeTarget()
		return "", nil, err
	}
	// Navigation has to commit to the extension origin before storage can be read.
	for i := 0; i < 40; i++ {
		var result struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		err = c.sessionCall(attached.ID, "Runtime.evaluate", map[string]string{"expression": "location.origin"}, &result)
		if err == nil && result.Result.Value == "chrome-extension://"+id {
			return attached.ID, closeTarget, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	closeTarget()
	return "", nil, fmt.Errorf("extension storage page did not initialize")
}
func storagePrefix(repo, area string) string { return "setting/" + repo + "/" + area + "/" }
func (a *App) capture(c *CDP, b Browser, e Extension, in Installed) error {
	if !e.Settings {
		return nil
	}
	session, closePage, err := c.storageSession(in.ID)
	if err != nil {
		return err
	}
	defer closePage()
	for _, area := range []string{"local", "sync"} {
		baselineKey := b.Name + "/" + e.Repo + "/" + area
		baseline, initialized := a.State.Baselines[baselineKey]
		if !initialized {
			continue
		}
		var result struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err = c.sessionCall(session, "Extensions.getStorageItems", map[string]string{"id": in.ID, "storageArea": area}, &result); err != nil {
			return err
		}
		prefix := storagePrefix(e.Repo, area)
		for key, value := range result.Data {
			if old, ok := baseline[key]; !ok || !equal(old, value) {
				a.put(prefix+url.PathEscape(key), value, false)
			}
		}
		for key := range baseline {
			if _, ok := result.Data[key]; !ok {
				a.put(prefix+url.PathEscape(key), nil, true)
			}
		}
	}
	return nil
}
func (a *App) restore(c *CDP, b Browser, e Extension, in Installed) error {
	if !e.Settings {
		return nil
	}
	session, closePage, err := c.storageSession(in.ID)
	if err != nil {
		return err
	}
	defer closePage()
	for _, area := range []string{"local", "sync"} {
		values := map[string]json.RawMessage{}
		deleted := []string{}
		prefix := storagePrefix(e.Repo, area)
		for key, r := range a.State.Records {
			if strings.HasPrefix(key, prefix) {
				decoded, err := url.PathUnescape(strings.TrimPrefix(key, prefix))
				if err != nil {
					return err
				}
				if r.Deleted {
					deleted = append(deleted, decoded)
				} else {
					values[decoded] = r.Value
				}
			}
		}
		if len(values) > 0 {
			if err = c.sessionCall(session, "Extensions.setStorageItems", map[string]any{"id": in.ID, "storageArea": area, "values": values}, nil); err != nil {
				return err
			}
		}
		if len(deleted) > 0 {
			if err = c.sessionCall(session, "Extensions.removeStorageItems", map[string]any{"id": in.ID, "storageArea": area, "keys": deleted}, nil); err != nil {
				return err
			}
		}
		var current struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err = c.sessionCall(session, "Extensions.getStorageItems", map[string]string{"id": in.ID, "storageArea": area}, &current); err != nil {
			return err
		}
		// Initial local settings are uploaded only after remote settings have been applied.
		for key, value := range current.Data {
			recordKey := prefix + url.PathEscape(key)
			if _, ok := a.State.Records[recordKey]; !ok {
				a.put(recordKey, value, false)
			}
		}
		a.State.Baselines[b.Name+"/"+e.Repo+"/"+area] = current.Data
	}
	return nil
}
