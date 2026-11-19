package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/zalando/go-keyring"
)

// bootstrapSync accepts credentials on stdin so they never appear in process arguments.
func (a *App) bootstrapSync() error {
	var creds credentials
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 16384)).Decode(&creds); err != nil {
		return err
	}
	if creds.Token == "" {
		return fmt.Errorf("GitHub token is required")
	}
	d := desktopState{root: a.Root}
	if prior, err := d.savedCredentials(); err == nil {
		var saved credentials
		if json.Unmarshal([]byte(prior), &saved) == nil && creds.Password == "" {
			creds.Password = saved.Password
		}
	}
	if creds.Password == "" {
		creds.Password = randomID() + randomID()
	}
	encoded, _ := json.Marshal(creds)
	if err := keyring.Set(d.credentialService(), "gist", string(encoded)); err != nil {
		return err
	}
	os.Setenv("ADDONBRIDGE_GITHUB_TOKEN", creds.Token)
	os.Setenv("ADDONBRIDGE_PASSWORD", creds.Password)
	if a.Config.Gist == "" {
		content, err := seal(Snapshot{1, a.State.Records}, creds.Password)
		if err != nil {
			return err
		}
		body := map[string]any{"description": "AddonBridge encrypted extension catalog and settings", "public": false, "files": map[string]any{syncFilePrefix + a.Config.Device + ".json": map[string]string{"content": content}}}
		data, err := request("POST", githubAPI+"/gists", body, true)
		if err != nil {
			return err
		}
		var result struct {
			ID string `json:"id"`
		}
		if err = json.Unmarshal(data, &result); err != nil {
			return err
		}
		if result.ID == "" {
			return fmt.Errorf("GitHub returned no Gist ID")
		}
		a.Config.Gist = result.ID
		if err = a.saveConfig(); err != nil {
			return err
		}
	}
	if err := a.pull(); err != nil {
		return err
	}
	if err := a.save(); err != nil {
		return err
	}
	if err := a.push(); err != nil {
		return err
	}
	fmt.Println("https://gist.github.com/" + a.Config.Gist)
	return nil
}
