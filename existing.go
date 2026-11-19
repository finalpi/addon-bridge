package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

type ExistingInstall struct {
	RegisteredRevision string    `json:"registeredRevision,omitempty"`
	Status             string    `json:"status,omitempty"`
	Repo               string    `json:"repo"`
	Browser            string    `json:"browser"`
	Profile            string    `json:"profile"`
	Path               string    `json:"path"`
	ID                 string    `json:"id"`
	Revision           string    `json:"revision"`
	ActiveRevision     string    `json:"activeRevision"`
	Seen               time.Time `json:"seen"`
}
type BridgeConfig struct {
	Address string `json:"address"`
	Secret  string `json:"secret"`
}

func listenDesktop(root string) (net.Listener, error) {
	file := filepath.Join(root, "bridge-server.json")
	var cfg BridgeConfig
	err := readJSON(file, &cfg)
	if err == nil {
		host, _, err := net.SplitHostPort(cfg.Address)
		if err != nil || host != "127.0.0.1" || len(cfg.Secret) != 64 {
			return nil, fmt.Errorf("invalid local bridge configuration")
		}
		return net.Listen("tcp4", cfg.Address)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	cfg = BridgeConfig{listener.Addr().String(), randomID() + randomID()}
	if err = atomicJSON(file, cfg); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}
func bridgeToken(secret, key string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}
func existingKey(b Browser, repo string) string { return b.Name + "@" + b.Profile + "/" + repo }
func extensionID(key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	id := make([]byte, 32)
	for i := 0; i < 16; i++ {
		id[i*2] = 'a' + sum[i]>>4
		id[i*2+1] = 'a' + sum[i]&15
	}
	return string(id), nil
}
func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0600)
	})
}
func addPermission(manifest map[string]any, field, value string) {
	values, _ := manifest[field].([]any)
	for _, v := range values {
		if v == value {
			return
		}
	}
	manifest[field] = append(values, value)
}
func allowBridgeConnection(manifest map[string]any) {
	csp, ok := manifest["content_security_policy"].(map[string]any)
	if !ok {
		return
	}
	policy, ok := csp["extension_pages"].(string)
	if !ok {
		return
	}
	directives := strings.Split(policy, ";")
	fallback := ""
	for i, line := range directives {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "connect-src" {
			directives[i] = strings.ReplaceAll(line, "'none'", "") + " http://127.0.0.1:*"
			csp["extension_pages"] = strings.Join(directives, ";")
			return
		}
		if fields[0] == "default-src" {
			fallback = strings.Join(fields[1:], " ")
		}
	}
	if fallback != "" {
		csp["extension_pages"] = policy + "; connect-src " + strings.ReplaceAll(fallback, "'none'", "") + " http://127.0.0.1:*"
	}
}
func (a *App) prepareExisting(b Browser, e Extension, source, revision string) error {
	if !validProfile(b.Name, b.Profile) {
		return fmt.Errorf("%s: 所选现有资料不存在，请在浏览器页面重新选择", b.Name)
	}
	return a.prepareExistingAt(b, e, source, revision)
}
func (a *App) prepareExistingAt(b Browser, e Extension, source, revision string) error {
	var cfg BridgeConfig
	if err := readJSON(filepath.Join(a.Root, "bridge-server.json"), &cfg); err != nil {
		return fmt.Errorf("请先启动托盘程序以启用现有资料连接: %w", err)
	}
	key := existingKey(b, e.Repo)
	revision += "/bridge-v2"
	dest := filepath.Join(a.Root, "existing", b.Name, b.Profile, repoName(e.Repo))
	old := a.State.Existing[key]
	if old.Revision == revision {
		if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err == nil {
			return nil
		}
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = copyTree(source, stage); err != nil {
		return err
	}
	var manifest map[string]any
	if err = readJSON(filepath.Join(stage, "manifest.json"), &manifest); err != nil {
		return err
	}
	manifestKey, ok := manifest["key"].(string)
	if !ok {
		return fmt.Errorf("missing stable extension key")
	}
	id, err := extensionID(manifestKey)
	if err != nil {
		return err
	}
	addPermission(manifest, "permissions", "alarms")
	addPermission(manifest, "permissions", "storage")
	addPermission(manifest, "host_permissions", "http://127.0.0.1/*")
	allowBridgeConnection(manifest)
	background, _ := manifest["background"].(map[string]any)
	if background == nil {
		background = map[string]any{}
	}
	original, _ := background["service_worker"].(string)
	dir := filepath.Dir(original)
	if original == "" {
		dir = "."
	}
	if !filepath.IsLocal(dir) {
		return fmt.Errorf("invalid background worker path")
	}
	wrapper := filepath.Join(dir, bridgeWorker)
	if _, err = os.Stat(filepath.Join(stage, wrapper)); err == nil {
		return fmt.Errorf("reserved worker filename exists")
	}
	code := ""
	if original != "" {
		name, _ := json.Marshal("./" + filepath.Base(original))
		if background["type"] == "module" {
			code = "import " + string(name) + ";\n"
		} else {
			code = "importScripts(" + string(name) + ");\n"
		}
	}
	agent, err := desktopFiles.ReadFile("ui/extension-bridge.js")
	if err != nil {
		return err
	}
	settings, _ := json.Marshal(map[string]string{"url": "http://" + cfg.Address + "/bridge/" + base64.RawURLEncoding.EncodeToString([]byte(key)), "token": bridgeToken(cfg.Secret, key), "revision": revision})
	code += "\n;(()=>{ const config=" + string(settings) + ";\n" + string(agent) + "\n})();\n"
	if err = os.WriteFile(filepath.Join(stage, wrapper), []byte(code), 0600); err != nil {
		return err
	}
	background["service_worker"] = filepath.ToSlash(wrapper)
	manifest["background"] = background
	if err = atomicJSON(filepath.Join(stage, "manifest.json"), manifest); err != nil {
		return err
	}
	backup := dest + ".previous"
	// Recover a swap interrupted after moving the old directory.
	if _, err = os.Stat(dest); os.IsNotExist(err) {
		if _, err = os.Stat(backup); err == nil {
			if err = os.Rename(backup, dest); err != nil {
				return err
			}
		}
	}
	if err = os.RemoveAll(backup); err != nil {
		return err
	}
	existed := false
	if _, err = os.Stat(dest); err == nil {
		if err = os.Rename(dest, backup); err != nil {
			return err
		}
		existed = true
	}
	if err = os.Rename(stage, dest); err != nil {
		if existed {
			os.Rename(backup, dest)
		}
		return err
	}
	if a.State.Existing == nil {
		a.State.Existing = map[string]ExistingInstall{}
	}
	a.State.Existing[key] = ExistingInstall{Repo: e.Repo, Browser: b.Name, Profile: b.Profile, Path: dest, ID: id, Revision: revision, ActiveRevision: old.ActiveRevision, Seen: old.Seen, RegisteredRevision: old.RegisteredRevision, Status: "awaiting_browser_load"}
	return nil
}

type BridgeMessage struct {
	Action   string                                `json:"action"`
	Revision string                                `json:"revision"`
	Storage  map[string]map[string]json.RawMessage `json:"storage"`
}

func (d *desktopState) serveBridge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != d.address {
		http.Error(w, "Invalid host", 403)
		return
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(r.URL.Path, "/bridge/"))
	if err != nil {
		http.Error(w, "Invalid key", 400)
		return
	}
	key := string(keyBytes)
	a, err := openApp(d.root)
	if err != nil {
		http.Error(w, "Unavailable", 503)
		return
	}
	installed, ok := a.State.Existing[key]
	if !ok {
		http.Error(w, "Unknown extension", 404)
		return
	}
	origin := "chrome-extension://" + installed.ID
	if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin {
		http.Error(w, "Invalid extension origin", 403)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var cfg BridgeConfig
	if readJSON(filepath.Join(d.root, "bridge-server.json"), &cfg) != nil {
		http.Error(w, "Unavailable", 503)
		return
	}
	if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+bridgeToken(cfg.Secret, key))) {
		http.Error(w, "Unauthorized", 401)
		return
	}
	lock := flock.New(filepath.Join(d.root, "process.lock"))
	locked, err := lock.TryLock()
	if err != nil || !locked {
		http.Error(w, "Sync busy", 409)
		return
	}
	defer lock.Unlock()
	a, err = openApp(d.root)
	if err != nil {
		http.Error(w, "Unavailable", 503)
		return
	}
	installed = a.State.Existing[key]
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var msg BridgeMessage
	if json.NewDecoder(r.Body).Decode(&msg) != nil {
		http.Error(w, "Invalid message", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if record, ok := a.State.Records["ext/"+installed.Repo]; ok && record.Deleted {
		json.NewEncoder(w).Encode(map[string]bool{"remove": true})
		return
	}
	selected := false
	for _, b := range a.Config.Browsers {
		if b.Mode != "managed" && b.Name == installed.Browser && b.Profile == installed.Profile {
			selected = true
		}
	}
	if !selected {
		json.NewEncoder(w).Encode(map[string]bool{"inactive": true})
		return
	}
	e, ok := a.extensions()[installed.Repo]
	if !ok {
		http.Error(w, "Unknown extension", 404)
		return
	}
	installed.Seen = time.Now()
	installed.Status = "connected"
	installed.ActiveRevision = msg.Revision
	installed.RegisteredRevision = msg.Revision
	a.State.Existing[key] = installed
	response := map[string]any{"settings": e.Settings, "revision": installed.Revision}
	if e.Settings && (msg.Action == "exchange" || msg.Action == "ack") {
		if msg.Storage == nil {
			http.Error(w, "Missing storage", 400)
			return
		}
		desired := map[string]any{}
		for _, area := range []string{"local", "sync"} {
			current, ok := msg.Storage[area]
			if !ok || current == nil {
				http.Error(w, "Missing storage area", 400)
				return
			}
			baselineKey := "existing/" + key + "/" + area
			baseline, initialized := a.State.Baselines[baselineKey]
			prefix := storagePrefix(installed.Repo, area)
			if msg.Action == "ack" {
				a.State.Baselines[baselineKey] = current
				continue
			}
			for k, v := range current {
				if initialized {
					if prior, ok := baseline[k]; !ok || !equal(prior, v) {
						a.put(prefix+url.PathEscape(k), v, false)
					}
				} else {
					if _, exists := a.State.Records[prefix+url.PathEscape(k)]; !exists {
						a.put(prefix+url.PathEscape(k), v, false)
					}
				}
			}
			if initialized {
				for k := range baseline {
					if _, ok := current[k]; !ok {
						a.put(prefix+url.PathEscape(k), nil, true)
					}
				}
			}
			values := map[string]json.RawMessage{}
			removed := []string{}
			for k, record := range a.State.Records {
				if strings.HasPrefix(k, prefix) {
					name, err := url.PathUnescape(strings.TrimPrefix(k, prefix))
					if err != nil {
						continue
					}
					if record.Deleted {
						removed = append(removed, name)
					} else {
						values[name] = record.Value
					}
				}
			}
			desired[area] = map[string]any{"values": values, "removed": removed}
		}
		response["storage"] = desired
	}
	if err = a.save(); err != nil {
		http.Error(w, "State save failed", 500)
		return
	}
	json.NewEncoder(w).Encode(response)
}
