package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testApp(t *testing.T) *App {
	t.Helper()
	return &App{Root: t.TempDir(), Config: Config{Device: randomID()}, State: State{Schema: 1, Records: map[string]Record{}, Installed: map[string]Installed{}, Baselines: map[string]map[string]json.RawMessage{}, Releases: map[string]ReleaseStatus{}}}
}
func TestArchiveTraversal(t *testing.T) {
	if repoName("a--b/c") == repoName("a/b--c") {
		t.Fatal("repository cache paths collide")
	}
	for _, name := range []string{"../outside", "/absolute", "a\\..\\outside", "C:/outside"} {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, _ := w.Create(name)
		f.Write([]byte("bad"))
		w.Close()
		if err := extractZip(buf.Bytes(), t.TempDir()); err == nil {
			t.Fatal("accepted", name)
		}
	}
}
func TestEncryptionAndMerge(t *testing.T) {
	a := testApp(t)
	b := testApp(t)
	a.put("ext/test/tool", Extension{Repo: "test/tool", Asset: "*.zip"}, false)
	encoded, err := seal(Snapshot{1, a.State.Records}, "a-test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unseal(encoded, "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	snap, err := unseal(encoded, "a-test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if err = b.merge(snap.Records); err != nil {
		t.Fatal(err)
	}
	b.put("ext/test/tool", nil, true)
	if err = a.merge(b.State.Records); err != nil {
		t.Fatal(err)
	}
	if len(a.extensions()) != 0 {
		t.Fatal("deletion lost")
	}
	a.merge(snap.Records)
	if len(a.extensions()) != 0 {
		t.Fatal("old device resurrected deletion")
	}
}
func TestGistTwoDevices(t *testing.T) {
	files := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing authentication")
		}
		if r.Method == "PATCH" {
			var body struct {
				Files map[string]map[string]any `json:"files"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for k, v := range body.Files {
				files[k] = v
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"files": files})
	}))
	defer server.Close()
	old := githubAPI
	githubAPI = server.URL
	defer func() { githubAPI = old }()
	t.Setenv("ADDONBRIDGE_GITHUB_TOKEN", "test-token")
	t.Setenv("ADDONBRIDGE_PASSWORD", "a-test-password-123")
	a, b := testApp(t), testApp(t)
	a.Config.Gist = "abcd"
	b.Config.Gist = "abcd"
	a.put("ext/test/first", Extension{Repo: "test/first", Asset: "*.zip"}, false)
	b.put("ext/test/second", Extension{Repo: "test/second", Asset: "*.zip"}, false)
	for _, app := range []*App{a, b} {
		if err := app.push(); err != nil {
			t.Fatal(err)
		}
	}
	for _, app := range []*App{a, b} {
		if err := app.pull(); err != nil {
			t.Fatal(err)
		}
		if len(app.extensions()) != 2 {
			t.Fatal("lost concurrent update")
		}
	}
}
func fixture(t *testing.T, dir, version string) {
	t.Helper()
	os.MkdirAll(dir, 0700)
	manifest := map[string]any{"manifest_version": 3, "name": "AddonBridge Test", "version": version, "permissions": []string{"storage"}, "key": "MCowBQYDK2VwAyEAaKOwDb+iBnEGXquCKHSEWYOgPE21dLcG0YRKPkTaqZo="}
	if err := atomicJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, bridgePage), []byte("<!doctype html><title>Test</title>"), 0600)
}
func TestBrowserIntegration(t *testing.T) {
	if os.Getenv("ADDONBRIDGE_INTEGRATION") != "1" {
		t.Skip("set ADDONBRIDGE_INTEGRATION=1 to test installed browsers")
	}
	for _, name := range []string{"chrome", "brave"} {
		t.Run(name, func(t *testing.T) {
			exe, err := browserExecutable(name)
			if err != nil {
				t.Fatal(err)
			}
			a := testApp(t)
			b := Browser{Name: name, Executable: exe, Mode: "managed"}
			c, err := a.launch(b, true)
			if err != nil {
				t.Fatal(err)
			}
			defer c.socket.Close()
			defer c.call("Browser.close", map[string]any{}, nil)
			dir := filepath.Join(a.Root, "fixture1")
			fixture(t, dir, "1.0.0")
			var loaded struct {
				ID string `json:"id"`
			}
			if err = c.call("Extensions.loadUnpacked", map[string]string{"path": dir}, &loaded); err != nil {
				t.Fatal(err)
			}
			id := loaded.ID
			e := Extension{Repo: "test/tool", Asset: "*.zip", Settings: true}
			in := Installed{ID: id, Path: dir}
			a.put("ext/"+e.Repo, e, false)
			a.put(storagePrefix(e.Repo, "local")+"theme", "dark", false)
			if err = a.restore(c, b, e, in); err != nil {
				t.Fatal(err)
			}
			session, closePage, err := c.storageSession(id)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.sessionCall(session, "Extensions.setStorageItems", map[string]any{"id": id, "storageArea": "local", "values": map[string]any{"theme": "light", "count": 42}}, nil); err != nil {
				t.Fatal(err)
			}
			closePage()
			if err = a.capture(c, b, e, in); err != nil {
				t.Fatal(err)
			}
			if string(a.State.Records[storagePrefix(e.Repo, "local")+"theme"].Value) != `"light"` {
				t.Fatal("capture failed")
			}
			dir2 := filepath.Join(a.Root, "fixture2")
			fixture(t, dir2, "2.0.0")
			if err = c.call("Extensions.loadUnpacked", map[string]string{"path": dir2}, &loaded); err != nil {
				t.Fatal(err)
			}
			if loaded.ID != id {
				t.Fatal("extension identity changed")
			}
			session, closePage, err = c.storageSession(id)
			if err != nil {
				t.Fatal(err)
			}
			var values struct {
				Data map[string]any `json:"data"`
			}
			if err = c.sessionCall(session, "Extensions.getStorageItems", map[string]string{"id": id, "storageArea": "local"}, &values); err != nil {
				t.Fatal(err)
			}
			closePage()
			if values.Data["theme"] != "light" || values.Data["count"] != float64(42) {
				t.Fatalf("settings lost during update: %v", values.Data)
			}
			a.put(storagePrefix(e.Repo, "local")+"count", nil, true)
			if err = a.restore(c, b, e, in); err != nil {
				t.Fatal(err)
			}
			if _, ok := a.State.Baselines[name+"/test/tool/local"]["count"]; ok {
				t.Fatal("settings deletion failed")
			}
			fmt.Println(name, "install, version update, settings capture/restore and deletion passed")
		})
	}
}

func TestReleaseEndToEnd(t *testing.T) {
	if os.Getenv("ADDONBRIDGE_RELEASE_TEST") != "1" {
		t.Skip("set ADDONBRIDGE_RELEASE_TEST=1 for live GitHub release verification")
	}
	a := testApp(t)
	e := releaseFixture(t)
	a.put("ext/"+e.Repo, e, false)
	for _, name := range []string{"chrome", "brave"} {
		exe, err := browserExecutable(name)
		if err != nil {
			t.Fatal(err)
		}
		b := Browser{Name: name, Executable: exe, Mode: "managed"}
		a.Config.Browsers = append(a.Config.Browsers, b)
		c, err := a.launch(b, true)
		if err != nil {
			t.Fatal(err)
		}
		defer c.socket.Close()
		defer c.call("Browser.close", map[string]any{}, nil)
	}
	if err := a.cycle(); err != nil {
		t.Fatal(err)
	}
	first := a.State.Installed["chrome/"+e.Repo]
	second := a.State.Installed["brave/"+e.Repo]
	if first.ID == "" || first.ID != second.ID {
		t.Fatal("cross-browser extension identity mismatch")
	}
	c, err := a.connect(a.Config.Browsers[0])
	if err != nil {
		t.Fatal(err)
	}
	defer c.socket.Close()
	session, closePage, err := c.storageSession(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.sessionCall(session, "Extensions.setStorageItems", map[string]any{"id": first.ID, "storageArea": "local", "values": map[string]any{"addonbridge_test": "cross-browser"}}, nil); err != nil {
		t.Fatal(err)
	}
	closePage()
	if err = a.cycle(); err != nil {
		t.Fatal(err)
	}
	value := a.State.Baselines["brave/"+e.Repo+"/local"]["addonbridge_test"]
	if string(value) != `"cross-browser"` {
		t.Fatal("Chrome to Brave settings sync failed")
	}
	t.Log("Configured release installed; settings synchronized through the normal sync cycle")
}

func releaseFixture(t *testing.T) Extension {
	t.Helper()
	repo := os.Getenv("ADDONBRIDGE_TEST_REPOSITORY")
	asset := os.Getenv("ADDONBRIDGE_TEST_ASSET")
	if repo == "" || asset == "" {
		t.Skip("set ADDONBRIDGE_TEST_REPOSITORY and ADDONBRIDGE_TEST_ASSET for live release tests")
	}
	if !repoPattern.MatchString(repo) {
		t.Fatal("invalid test repository")
	}
	return Extension{Repo: repo, Asset: asset, Settings: true}
}

func TestDefaultChromeAssetSelection(t *testing.T) {
	oldAPI := githubAPI
	defer func() { githubAPI = oldAPI }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v2.2.6", "assets": []map[string]string{
			{"name": "tool_v2.2.6_chrome.zip", "browser_download_url": "https://github.com/example/tool/releases/download/v2.2.6/tool_v2.2.6_chrome.zip"},
			{"name": "tool_v2.2.6_edge.zip", "browser_download_url": "https://github.com/example/tool/releases/download/v2.2.6/tool_v2.2.6_edge.zip"},
			{"name": "tool_v2.2.6_firefox.zip", "browser_download_url": "https://github.com/example/tool/releases/download/v2.2.6/tool_v2.2.6_firefox.zip"},
			{"name": "tool_v2.2.6_source.zip", "browser_download_url": "https://github.com/example/tool/archive/v2.2.6.zip"},
		}})
	}))
	defer server.Close()
	githubAPI = server.URL
	a := testApp(t)
	selected, err := a.selectRelease(Extension{Repo: "example/tool"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Asset != "tool_v2.2.6_chrome.zip" {
		t.Fatalf("selected %q", selected.Asset)
	}
}
