package main

import (
	"path/filepath"
	"strings"
)

func preferredLanguage(root string) (string, string) {
	var prefs struct {
		Language string `json:"language"`
	}
	readJSON(filepath.Join(root, "language.json"), &prefs)
	if prefs.Language == "en" || prefs.Language == "zh-CN" {
		return prefs.Language, prefs.Language
	}
	if strings.HasPrefix(strings.ToLower(systemLanguage()), "zh") {
		return "zh-CN", "auto"
	}
	return "en", "auto"
}
func (d *desktopState) text(en, zh string) string {
	lang, _ := preferredLanguage(d.root)
	if lang == "zh-CN" {
		return zh
	}
	return en
}
