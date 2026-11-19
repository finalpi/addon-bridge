package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 120 * time.Second}
var githubAPI = "https://api.github.com"

func request(method, address string, body any, auth bool) ([]byte, error) {
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, address, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AddonBridge/0.4")
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		if token := osToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s returned HTTP %d", method, req.URL.Host, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if len(data) >= 64<<20 {
		return nil, fmt.Errorf("response exceeds limit")
	}
	return data, nil
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

type selectedRelease struct {
	Tag, Asset, Digest, URL, Revision string
}

func (a *App) selectRelease(e Extension) (selectedRelease, error) {
	data, err := request("GET", githubAPI+"/repos/"+e.Repo+"/releases/latest", nil, true)
	if err != nil {
		return selectedRelease{}, err
	}
	var r release
	if err = json.Unmarshal(data, &r); err != nil {
		return selectedRelease{}, err
	}
	pattern := e.Asset
	if pattern == "*.zip" {
		pattern = ""
	}
	assets := r.Assets
	if pattern != "" {
		filtered := assets[:0]
		for _, asset := range assets {
			matched, matchErr := path.Match(pattern, asset.Name)
			if matchErr != nil {
				return selectedRelease{}, matchErr
			}
			if matched {
				filtered = append(filtered, asset)
			}
		}
		assets = filtered
	} else {
		filtered := assets[:0]
		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if !strings.HasSuffix(name, ".zip") || strings.Contains(name, "source") || strings.Contains(name, "firefox") || strings.Contains(name, "safari") {
				continue
			}
			filtered = append(filtered, asset)
		}
		assets = filtered
		preferred := assets[:0]
		for _, asset := range assets {
			name := strings.ToLower(asset.Name)
			if strings.Contains(name, "chrome") || strings.Contains(name, "chromium") || strings.Contains(name, "extension") {
				preferred = append(preferred, asset)
			}
		}
		if len(preferred) > 0 {
			assets = preferred
		}
	}
	if len(assets) != 1 {
		names := make([]string, 0, len(assets))
		for _, asset := range assets {
			names = append(names, asset.Name)
		}
		return selectedRelease{}, fmt.Errorf("%s: automatic asset detection found %d candidates (%s); edit the asset pattern", e.Repo, len(assets), strings.Join(names, ", "))
	}
	asset := assets[0]
	return selectedRelease{Tag: r.Tag, Asset: asset.Name, Digest: asset.Digest, URL: asset.URL, Revision: r.Tag + "/" + asset.Name + "/" + asset.Digest}, nil
}

func (a *App) checkLatest(e Extension) error {
	if a.State.Releases == nil {
		a.State.Releases = map[string]ReleaseStatus{}
	}
	selected, err := a.selectRelease(e)
	status := ReleaseStatus{CheckedAt: time.Now()}
	if err != nil {
		status.Error = err.Error()
		a.State.Releases[e.Repo] = status
		return err
	}
	status.LatestVersion = selected.Tag
	status.LatestAsset = selected.Asset
	status.LatestRevision = selected.Revision
	a.State.Releases[e.Repo] = status
	return nil
}

func preparePackage(a *App, e Extension) (string, string, error) {
	if a.State.Releases == nil {
		a.State.Releases = map[string]ReleaseStatus{}
	}
	selected, err := a.selectRelease(e)
	if err != nil {
		a.State.Releases[e.Repo] = ReleaseStatus{CheckedAt: time.Now(), Error: err.Error()}
		return "", "", err
	}
	a.State.Releases[e.Repo] = ReleaseStatus{LatestVersion: selected.Tag, LatestAsset: selected.Asset, LatestRevision: selected.Revision, CheckedAt: time.Now()}
	address, digest := selected.URL, selected.Digest
	revision := selected.Revision
	hash := sha256.Sum256([]byte(revision))
	dir := filepath.Join(a.Root, "packages", repoName(e.Repo), hex.EncodeToString(hash[:12]))
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
		return dir, revision, nil
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" {
		return "", "", fmt.Errorf("release asset must be hosted on github.com")
	}
	data, err := request("GET", address, nil, false)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	if digest != "" && digest != "sha256:"+hex.EncodeToString(sum[:]) {
		return "", "", fmt.Errorf("release digest mismatch")
	}
	parent := filepath.Dir(dir)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", "", err
	}
	stage, err := os.MkdirTemp(parent, ".stage-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(stage)
	if err = extractZip(data, stage); err != nil {
		return "", "", err
	}
	var manifests []string
	err = filepath.WalkDir(stage, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "manifest.json" {
			manifests = append(manifests, p)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if len(manifests) != 1 {
		return "", "", fmt.Errorf("package must contain exactly one manifest.json, found %d", len(manifests))
	}
	source := filepath.Dir(manifests[0])
	var manifest map[string]any
	if err = readJSON(manifests[0], &manifest); err != nil {
		return "", "", err
	}
	if manifest["manifest_version"] != float64(3) {
		return "", "", fmt.Errorf("only Manifest V3 packages are supported")
	}
	if manifest["key"] == nil {
		seed := sha256.Sum256([]byte(extensionKeyDomain + e.Repo))
		public := ed25519.NewKeyFromSeed(seed[:]).Public()
		der, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			return "", "", err
		}
		manifest["key"] = base64.StdEncoding.EncodeToString(der)
	}
	// A local extension page provides an extension-origin target for storage APIs.
	if _, err = os.Stat(filepath.Join(source, bridgePage)); err == nil {
		return "", "", fmt.Errorf("reserved bridge filename is already present")
	}
	if err = os.WriteFile(filepath.Join(source, bridgePage), []byte("<!doctype html><meta charset=utf-8><title>AddonBridge</title>"), 0600); err != nil {
		return "", "", err
	}
	if err = atomicJSON(manifests[0], manifest); err != nil {
		return "", "", err
	}
	if err = os.Rename(source, dir); err != nil {
		return "", "", err
	}
	return dir, revision, nil
}

func extractZip(data []byte, dest string) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(z.File) > 20000 {
		return fmt.Errorf("too many archive entries")
	}
	var total uint64
	for _, f := range z.File {
		clean := path.Clean(f.Name)
		if strings.Contains(f.Name, "\\") || strings.Contains(f.Name, ":") || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe archive path %q", f.Name)
		}
		if f.UncompressedSize64 > (256<<20)-total {
			return fmt.Errorf("unpacked archive exceeds 256 MiB")
		}
		total += f.UncompressedSize64
		target := filepath.Join(dest, filepath.FromSlash(clean))
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		input, err := f.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, int64(f.UncompressedSize64)+1))
		input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
