package main

import (
	"encoding/json"
	"github.com/gofrs/flock"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Default", "Profile 1", "System Profile"} {
		os.MkdirAll(filepath.Join(root, dir), 0700)
		atomicJSON(filepath.Join(root, dir, "Preferences"), map[string]any{})
	}
	atomicJSON(filepath.Join(root, "Local State"), map[string]any{"profile": map[string]any{"last_used": "Profile 1", "info_cache": map[string]any{"Profile 1": map[string]string{"name": "Personal"}}}})
	profiles, selected := discoverProfiles(root)
	if len(profiles) != 2 || selected != "Profile 1" || profiles[1].Name != "Personal" {
		t.Fatalf("bad profile selection: %v %s", profiles, selected)
	}
}
func existingFixture(t *testing.T, browser string) (*App, Browser, Extension, func()) {
	t.Helper()
	a := testApp(t)
	exe, err := browserExecutable(browser)
	if err != nil {
		t.Fatal(err)
	}
	b := Browser{Name: browser, Executable: exe, Mode: "existing", Profile: "Default"}
	a.Config.Browsers = []Browser{b}
	e := Extension{Repo: "test/tool", Asset: "*.zip", Settings: true}
	a.put("ext/"+e.Repo, e, false)
	if err = a.saveConfig(); err != nil {
		t.Fatal(err)
	}
	if err = a.save(); err != nil {
		t.Fatal(err)
	}
	listener, err := listenDesktop(a.Root)
	if err != nil {
		t.Fatal(err)
	}
	d := &desktopState{root: a.Root, address: listener.Addr().String()}
	server := &http.Server{Handler: http.HandlerFunc(d.serve)}
	go server.Serve(listener)
	source := filepath.Join(a.Root, "source")
	menuFixture(t, source, "1.0.0.1")
	if err = a.prepareExistingAt(b, e, source, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err = a.save(); err != nil {
		t.Fatal(err)
	}
	return a, b, e, func() { server.Close() }
}

func menuFixture(t *testing.T, dir, version string) {
	t.Helper()
	fixture(t, dir, version)
	manifestPath := filepath.Join(dir, "manifest.json")
	var manifest map[string]any
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["permissions"] = []string{"storage", "contextMenus"}
	manifest["background"] = map[string]string{"service_worker": "background.js"}
	if err := atomicJSON(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	const worker = `chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({id:'image-menu', title:'Image action', contexts:['image']}, () => {
      if (!chrome.runtime.lastError) chrome.storage.local.set({menuVersion: chrome.runtime.getManifest().version});
    });
  });
});`
	if err := os.WriteFile(filepath.Join(dir, "background.js"), []byte(worker), 0600); err != nil {
		t.Fatal(err)
	}
}

func requireImageMenu(t *testing.T, c *CDP, session string) {
	t.Helper()
	var result struct {
		Result struct{ Value string }
	}
	expression := `new Promise(resolve => chrome.contextMenus.update('image-menu', {title:'Image action'}, () => resolve(chrome.runtime.lastError?.message ?? 'registered')))`
	for i := 0; i < 20; i++ {
		if err := c.sessionCall(session, "Runtime.evaluate", map[string]any{"expression": expression, "awaitPromise": true, "returnByValue": true}, &result); err != nil {
			t.Fatal(err)
		}
		if result.Result.Value == "registered" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("image menu was not registered: %s", result.Result.Value)
}
func waitExisting(t *testing.T, a *App, key, revision string) {
	t.Helper()
	for i := 0; i < 400; i++ {
		latest, err := openApp(a.Root)
		if err == nil {
			entry := latest.State.Existing[key]
			if entry.ActiveRevision == revision && !entry.Seen.IsZero() {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	latest, _ := openApp(a.Root)
	t.Fatalf("bridge expected %s, current %+v", revision, latest.State.Existing[key])
}
func TestExistingBridgeIntegration(t *testing.T) {
	if os.Getenv("ADDONBRIDGE_INTEGRATION") != "1" {
		t.Skip("set ADDONBRIDGE_INTEGRATION=1")
	}
	for _, browser := range []string{"chrome", "brave"} {
		t.Run(browser, func(t *testing.T) {
			a, b, e, cleanup := existingFixture(t, browser)
			defer cleanup()
			c, err := a.launch(b, true)
			if err != nil {
				t.Fatal(err)
			}
			defer stopTestBrowser(t, a, b, c)
			key := existingKey(b, e.Repo)
			entry := a.State.Existing[key]
			if err = c.call("Extensions.loadUnpacked", map[string]string{"path": entry.Path}, nil); err != nil {
				t.Fatal(err)
			}
			waitExisting(t, a, key, entry.Revision)
			session, closePage, err := c.storageSession(entry.ID)
			if err != nil {
				t.Fatal(err)
			}
			requireImageMenu(t, c, session)
			stateLock := flock.New(filepath.Join(a.Root, "process.lock"))
			if err := stateLock.Lock(); err != nil {
				t.Fatal(err)
			}
			latest, err := openApp(a.Root)
			if err != nil {
				t.Fatal(err)
			}
			latest.put(storagePrefix(e.Repo, "local")+"theme", "green", false)
			source := filepath.Join(a.Root, "source2")
			menuFixture(t, source, "1.0.0")
			if err = latest.prepareExistingAt(b, e, source, "2.0.0"); err != nil {
				t.Fatal(err)
			}
			if err = latest.save(); err != nil {
				t.Fatal(err)
			}
			stateLock.Unlock()
			closePage()
			if err = c.call("Extensions.loadUnpacked", map[string]string{"path": entry.Path}, nil); err != nil {
				t.Fatal(err)
			}
			waitExisting(t, a, key, "2.0.0/bridge-v2")
			session, closePage, err = c.storageSession(entry.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer closePage()
			requireImageMenu(t, c, session)
			var result struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err = c.sessionCall(session, "Extensions.getStorageItems", map[string]string{"id": entry.ID, "storageArea": "local"}, &result); err != nil {
				t.Fatal(err)
			}
			if string(result.Data["theme"]) != `"green"` {
				t.Fatalf("settings not restored: %s", result.Data["theme"])
			}
			if string(result.Data["menuVersion"]) != `"1.0.0"` {
				t.Fatalf("menu initialization did not run with the restored manifest version: %s", result.Data["menuVersion"])
			}
		})
	}
}
