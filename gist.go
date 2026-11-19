package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

type Envelope struct {
	Schema int    `json:"schema"`
	Salt   string `json:"salt"`
	Nonce  string `json:"nonce"`
	Data   string `json:"data"`
}
type Snapshot struct {
	Schema  int               `json:"schema"`
	Records map[string]Record `json:"records"`
}

func seal(value any, password string) (string, error) {
	if len(password) < 12 {
		return "", fmt.Errorf("ADDONBRIDGE_PASSWORD must contain at least 12 characters")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err = rand.Read(salt); err != nil {
		return "", err
	}
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	key := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	encode := base64.StdEncoding.EncodeToString
	encrypted := gcm.Seal(nil, nonce, raw, []byte(syncDomain))
	data, err := json.Marshal(Envelope{1, encode(salt), encode(nonce), encode(encrypted)})
	return string(data), err
}
func unseal(data, password string) (Snapshot, error) {
	var e Envelope
	var s Snapshot
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return s, err
	}
	if e.Schema != 1 {
		return s, fmt.Errorf("unsupported encryption schema")
	}
	salt, err := base64.StdEncoding.DecodeString(e.Salt)
	if err != nil || len(salt) != 16 {
		return s, fmt.Errorf("invalid salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(e.Nonce)
	if err != nil || len(nonce) != 12 {
		return s, fmt.Errorf("invalid nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(e.Data)
	if err != nil {
		return s, err
	}
	key := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	raw, err := gcm.Open(nil, nonce, ciphertext, []byte(syncDomain))
	if err != nil {
		return s, fmt.Errorf("Gist decryption failed: password mismatch or damaged data")
	}
	if err = json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	if s.Schema != 1 || s.Records == nil {
		return s, fmt.Errorf("invalid sync snapshot")
	}
	return s, nil
}
func (a *App) pull() error {
	if a.Config.Gist == "" {
		return nil
	}
	if osToken() == "" {
		return fmt.Errorf("set ADDONBRIDGE_GITHUB_TOKEN for Gist sync")
	}
	data, err := request("GET", githubAPI+"/gists/"+a.Config.Gist, nil, true)
	if err != nil {
		return err
	}
	var gist struct {
		Files map[string]struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
		} `json:"files"`
	}
	if err = json.Unmarshal(data, &gist); err != nil {
		return err
	}
	if _, ok := gist.Files[syncFilePrefix+a.Config.Device+".json"]; !ok {
		a.State.UploadedHash = ""
	}
	// Validate all files before changing local state.
	snapshots := []Snapshot{}
	for name, file := range gist.Files {
		if !strings.HasPrefix(name, syncFilePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		if file.Truncated {
			return fmt.Errorf("Gist file too large: %s", name)
		}
		snapshot, err := unseal(file.Content, password())
		if err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	for _, s := range snapshots {
		if err = a.merge(s.Records); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) push() error {
	if a.Config.Gist == "" {
		return nil
	}
	plain, err := json.Marshal(a.State.Records)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(append([]byte(a.Config.Gist), plain...))
	fingerprint := hex.EncodeToString(hash[:])
	if fingerprint == a.State.UploadedHash {
		return nil
	}
	encrypted, err := seal(Snapshot{1, a.State.Records}, password())
	if err != nil {
		return err
	}
	if len(encrypted) > 900000 {
		return fmt.Errorf("encrypted settings exceed Gist file limit; reduce selected settings")
	}
	name := syncFilePrefix + a.Config.Device + ".json"
	_, err = request("PATCH", githubAPI+"/gists/"+a.Config.Gist, map[string]any{"files": map[string]any{name: map[string]string{"content": encrypted}}}, true)
	if err != nil {
		return err
	}
	a.State.UploadedHash = fingerprint
	return a.save()
}
