package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Record struct {
	Counter uint64          `json:"counter"`
	Device  string          `json:"device"`
	Deleted bool            `json:"deleted,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}
type Extension struct {
	Repo     string `json:"repo"`
	Asset    string `json:"asset"`
	Settings bool   `json:"settings"`
}
type ReleaseStatus struct {
	LatestVersion  string    `json:"latest_version,omitempty"`
	LatestAsset    string    `json:"latest_asset,omitempty"`
	LatestRevision string    `json:"latest_revision,omitempty"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Error          string    `json:"error,omitempty"`
}
type Browser struct {
	Name       string `json:"name"`
	Executable string `json:"executable"`
	Mode       string `json:"mode,omitempty"`
	Profile    string `json:"profile,omitempty"`
}
type Config struct {
	Device   string    `json:"device"`
	Gist     string    `json:"gist,omitempty"`
	Browsers []Browser `json:"browsers"`
}
type Installed struct {
	Revision string `json:"revision"`
	Path     string `json:"path"`
	ID       string `json:"id"`
}
type State struct {
	Existing     map[string]ExistingInstall            `json:"existing,omitempty"`
	Releases     map[string]ReleaseStatus              `json:"releases,omitempty"`
	UploadedHash string                                `json:"uploaded_hash,omitempty"`
	Schema       int                                   `json:"schema"`
	Clock        uint64                                `json:"clock"`
	Records      map[string]Record                     `json:"records"`
	Installed    map[string]Installed                  `json:"installed"`
	Baselines    map[string]map[string]json.RawMessage `json:"baselines"`
}
type App struct {
	Root   string
	Config Config
	State  State
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var devicePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func atomicJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(name, path)
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func openApp(root string) (*App, error) {
	a := &App{Root: root}
	if err := readJSON(filepath.Join(root, "config.json"), &a.Config); err != nil {
		return nil, err
	}
	if !devicePattern.MatchString(a.Config.Device) {
		return nil, fmt.Errorf("invalid device ID")
	}
	a.State = State{Schema: 1, Records: map[string]Record{}, Installed: map[string]Installed{}, Baselines: map[string]map[string]json.RawMessage{}, Releases: map[string]ReleaseStatus{}}
	err := readJSON(filepath.Join(root, "state.json"), &a.State)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if a.State.Schema != 1 || a.State.Records == nil || a.State.Installed == nil || a.State.Baselines == nil {
		return nil, fmt.Errorf("unsupported or invalid state")
	}
	if a.State.Existing == nil {
		a.State.Existing = map[string]ExistingInstall{}
	}
	if a.State.Releases == nil {
		a.State.Releases = map[string]ReleaseStatus{}
	}
	for i := range a.Config.Browsers {
		b := &a.Config.Browsers[i]
		if b.Mode == "" {
			b.Mode = "existing"
		}
		if b.Profile == "" {
			b.Profile = defaultProfile(b.Name)
		}
	}
	return a, nil
}
func (a *App) save() error       { return atomicJSON(filepath.Join(a.Root, "state.json"), a.State) }
func (a *App) saveConfig() error { return atomicJSON(filepath.Join(a.Root, "config.json"), a.Config) }
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func canonical(v json.RawMessage) []byte {
	var x any
	if json.Unmarshal(v, &x) != nil {
		return v
	}
	b, _ := json.Marshal(x)
	return b
}
func equal(a, b json.RawMessage) bool { return bytes.Equal(canonical(a), canonical(b)) }
func (a *App) put(key string, value any, deleted bool) {
	data, _ := json.Marshal(value)
	old, ok := a.State.Records[key]
	if ok && old.Deleted == deleted && equal(old.Value, data) {
		return
	}
	a.State.Clock++
	a.State.Records[key] = Record{a.State.Clock, a.Config.Device, deleted, data}
}
func newer(x, y Record) bool {
	return x.Counter > y.Counter || (x.Counter == y.Counter && x.Device > y.Device)
}
func (a *App) merge(records map[string]Record) error {
	for key, r := range records {
		if !devicePattern.MatchString(r.Device) || r.Counter == 0 {
			return fmt.Errorf("invalid sync record")
		}
		if strings.HasPrefix(key, "ext/") {
			var e Extension
			if !r.Deleted {
				if json.Unmarshal(r.Value, &e) != nil || !repoPattern.MatchString(e.Repo) || key != "ext/"+e.Repo {
					return fmt.Errorf("invalid extension record")
				}
			}
		} else if !strings.HasPrefix(key, "setting/") && key != "interval" {
			return fmt.Errorf("unknown sync record %q", key)
		}
	}
	for key, r := range records {
		if r.Counter > a.State.Clock {
			a.State.Clock = r.Counter
		}
		if old, ok := a.State.Records[key]; !ok || newer(r, old) {
			a.State.Records[key] = r
		}
	}
	return nil
}
func (a *App) extensions() map[string]Extension {
	result := map[string]Extension{}
	for k, r := range a.State.Records {
		if strings.HasPrefix(k, "ext/") && !r.Deleted {
			var e Extension
			if json.Unmarshal(r.Value, &e) == nil {
				result[e.Repo] = e
			}
		}
	}
	return result
}
func repoName(repo string) string { return strings.ReplaceAll(repo, "/", "~") }
