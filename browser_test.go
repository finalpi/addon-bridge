package main

import (
	"testing"
	"time"
)

func stopTestBrowser(t *testing.T, a *App, b Browser, c *CDP) {
	t.Helper()
	c.call("Browser.close", map[string]any{}, nil)
	c.socket.Close()
	for i := 0; i < 40; i++ {
		probe, err := a.connect(b)
		if err != nil {
			time.Sleep(time.Second)
			return
		}
		probe.socket.Close()
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("test browser did not exit")
}
