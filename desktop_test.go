package main

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("ADDONBRIDGE_TEST_CHILD") == "1" && len(os.Args) > 1 && os.Args[1] == "sync" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestDesktopWorkerCompletesSync(t *testing.T) {
	t.Setenv("ADDONBRIDGE_TEST_CHILD", "1")
	d := &desktopState{root: t.TempDir(), jobs: make(chan []string, 1), stop: make(chan struct{})}
	go d.worker()
	defer close(d.stop)
	if !d.enqueue([]string{"sync"}) {
		t.Fatal("failed to queue sync")
	}
	for i := 0; i < 100; i++ {
		time.Sleep(50 * time.Millisecond)
		if d.mu.TryLock() {
			done := !d.busy
			failed := d.failed
			message := d.message
			d.mu.Unlock()
			if done {
				if failed {
					t.Fatal(message)
				}
				return
			}
		}
	}
	t.Fatal("desktop worker stalled while synchronizing")
}

func TestDesktopRequestBoundary(t *testing.T) {
	a := testApp(t)
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	if err := a.saveConfig(); err != nil {
		t.Fatal(err)
	}
	d := &desktopState{root: a.Root, token: "test-secret", address: "127.0.0.1:43210", jobs: make(chan []string, 1)}
	cases := []struct {
		name, host, origin, token, path, method, body string
		status                                        int
	}{
		{name: "static", host: d.address, path: "/", method: "GET", status: 200},
		{name: "unauthenticated", host: d.address, path: "/api/state", method: "GET", status: 401},
		{name: "rebinding", host: "evil.example", token: "test-secret", path: "/api/state", method: "GET", status: 403},
		{name: "cross-origin", host: d.address, origin: "https://evil.example", token: "test-secret", path: "/api/action", method: "POST", body: `{"action":"pause"}`, status: 403},
		{name: "authenticated", host: d.address, token: "test-secret", path: "/api/state", method: "GET", status: 200},
		{name: "queue", host: d.address, token: "test-secret", path: "/api/action", method: "POST", body: `{"action":"sync"}`, status: 200},
		{name: "busy", host: d.address, token: "test-secret", path: "/api/action", method: "POST", body: `{"action":"sync"}`, status: 409},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "http://"+c.host+c.path, strings.NewReader(c.body))
			r.Header.Set("Origin", c.origin)
			if c.token != "" {
				r.Header.Set("Authorization", "Bearer "+c.token)
			}
			w := httptest.NewRecorder()
			d.serve(w, r)
			if w.Code != c.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "test-secret") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestBrowserDoesNotInheritSyncCredentials(t *testing.T) {
	t.Setenv("ADDONBRIDGE_GITHUB_TOKEN", "test-secret")
	t.Setenv("ADDONBRIDGE_PASSWORD", "test-password")
	for _, entry := range browserEnvironment() {
		if strings.HasPrefix(strings.ToUpper(entry), "ADDONBRIDGE_GITHUB_TOKEN=") || strings.HasPrefix(strings.ToUpper(entry), "ADDONBRIDGE_PASSWORD=") {
			t.Fatal("sync credential would be passed to browser")
		}
	}
}
