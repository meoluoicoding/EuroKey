// Package config loads EuroKey's per-user settings on Linux.
//
// Files live in $XDG_CONFIG_HOME/eurokey/ (usually ~/.config/eurokey/):
//
//	eurokey.ini  — options, same key=value format as the Windows build
//	macro.txt    — "trigger:expansion" macros, auto-loaded by the engine
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"eurokey/engine"
)

// Dir is the per-user config directory.
func Dir() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(xdg, "eurokey")
}

func IniPath() string   { return filepath.Join(Dir(), "eurokey.ini") }
func MacroPath() string { return filepath.Join(Dir(), "macro.txt") }

// Settings mirrors the persisted eurokey.ini keys that apply on Linux.
type Settings struct {
	Enabled      bool
	Language     int // engine.LangOff / LangDE / LangFR / LangES
	MacroEnabled bool
	AutoCap      bool
	// ToggleKeys lists on/off toggle chords: "ctrl+shift", "alt+z",
	// "ctrl+space". Empty disables hotkey toggling.
	ToggleKeys []string
}

// LangChoice is one entry of the settings GUI language list.
type LangChoice struct {
	Label    string
	Language int
}

// LangChoices lists every language the engine offers.
func LangChoices() []LangChoice {
	return []LangChoice{
		{"Off", engine.LangOff},
		{"Deutsch", engine.LangDE},
		{"Français", engine.LangFR},
		{"Español", engine.LangES},
	}
}

// Defaults matches the Windows build's first-run defaults.
func Defaults() Settings {
	return Settings{
		Enabled:      true,
		Language:     engine.LangDE,
		MacroEnabled: true,
		AutoCap:      false,
		ToggleKeys:   []string{"ctrl+shift"},
	}
}

// Load reads eurokey.ini; a missing file returns defaults with ok=false.
func Load() (Settings, bool) {
	s := Defaults()
	m, err := readIni(IniPath())
	if err != nil {
		return s, false
	}
	getI := func(name string) (int, bool) {
		if v, ok := m[name]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				return n, true
			}
		}
		return 0, false
	}
	getB := func(name string, dflt bool) bool {
		if v, ok := m[name]; ok {
			return v == "1"
		}
		return dflt
	}
	if v, ok := getI("Enabled"); ok {
		s.Enabled = v != 0
	}
	if v, ok := getI("Language"); ok && v >= engine.LangOff && v <= engine.LangES {
		s.Language = v
	}
	s.MacroEnabled = getB("MacroEnabled", s.MacroEnabled)
	s.AutoCap = getB("AutoCap", s.AutoCap)
	s.ToggleKeys = toggleKeysFromIni(m)
	return s, true
}

// toggleKeysFromIni resolves the toggle chords. ToggleKeyNames is the
// Linux-owned key; a numeric ToggleKey is the Windows legacy value and must
// fall back to the defaults rather than silently disabling toggling.
func toggleKeysFromIni(m map[string]string) []string {
	if v, ok := m["ToggleKeyNames"]; ok {
		return parseToggleKeys(v) // explicit empty = disabled
	}
	if v, ok := m["ToggleKey"]; ok {
		if _, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return Defaults().ToggleKeys
		}
		return parseToggleKeys(v)
	}
	return Defaults().ToggleKeys
}

func parseToggleKeys(v string) []string {
	var keys []string
	for _, k := range strings.Split(v, ",") {
		k = strings.ToLower(strings.TrimSpace(k))
		switch k {
		case "ctrl+shift", "alt+z", "ctrl+space":
			keys = append(keys, k)
		}
	}
	return keys
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// EngineOptions maps the settings onto engine.Options.
func (s Settings) EngineOptions() engine.Options {
	return engine.Options{MacroEnabled: int32(b2i(s.MacroEnabled))}
}

// Save writes eurokey.ini atomically (temp + rename). Keys this build does not
// manage are preserved verbatim so a config shared with the Windows build
// keeps its settings.
func (s Settings) Save() error {
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		return err
	}
	bv := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	managed := [][2]string{
		{"Enabled", bv(s.Enabled)},
		{"Language", strconv.Itoa(s.Language)},
		{"MacroEnabled", bv(s.MacroEnabled)},
		{"AutoCap", bv(s.AutoCap)},
		{"ToggleKeyNames", strings.Join(s.ToggleKeys, ",")},
	}
	existing, _ := os.ReadFile(IniPath())
	content := mergeIni(string(existing), managed)

	tmp := IniPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, IniPath())
}

// mergeIni keeps every line of the existing ini the Linux settings do not
// manage (foreign keys, comments), then writes the managed keys fresh.
func mergeIni(existing string, managed [][2]string) string {
	managedSet := map[string]bool{}
	for _, kv := range managed {
		managedSet[kv[0]] = true
	}
	var sb strings.Builder
	sb.WriteString("[EuroKey]\n")
	for _, line := range strings.Split(existing, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "[") {
			continue
		}
		if t[0] == ';' || t[0] == '#' {
			sb.WriteString(line + "\n")
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if managedSet[key] {
			continue // rewritten below with the fresh value
		}
		if key == "ToggleKey" {
			if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
				continue // legacy chord list — superseded by ToggleKeyNames
			}
		}
		sb.WriteString(line + "\n")
	}
	for _, kv := range managed {
		sb.WriteString(kv[0] + "=" + kv[1] + "\n")
	}
	return sb.String()
}

// readIni parses a minimal key=value ini (one section, ; and # comments) —
// same format as the Windows shell's parser.
func readIni(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' || line[0] == '[' {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m, nil
}
