package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are remembered between program starts.
type Settings struct {
	FPS           int      `json:"fps"`
	Mode          PlayMode `json:"modus"`
	WhiteBG       bool     `json:"weisser_hintergrund"`
	LastOpenDir   string   `json:"zuletzt_geoeffnet"`
	LastParentDir string   `json:"projekte_ordner"`
}

func settingsPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "Sprite-Player", "einstellungen.json")
}

func loadSettings() Settings {
	s := Settings{FPS: 12}
	p := settingsPath()
	if p == "" {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

func saveSettings(s Settings) {
	p := settingsPath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o644)
}
