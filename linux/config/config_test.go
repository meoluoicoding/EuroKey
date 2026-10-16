package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eurokey/engine"
)

func TestDefaultsAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	s := Defaults()
	if s.Language != engine.LangDE || !s.Enabled || !s.MacroEnabled {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	if _, ok := Load(); ok {
		t.Fatal("Load with no ini must report ok=false")
	}

	s.Language = engine.LangES
	s.Enabled = false
	s.AutoCap = true
	s.ToggleKeys = []string{"ctrl+space"}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got, ok := Load()
	if !ok {
		t.Fatal("Load after Save must report ok=true")
	}
	if got.Language != engine.LangES || got.Enabled || !got.AutoCap ||
		len(got.ToggleKeys) != 1 || got.ToggleKeys[0] != "ctrl+space" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// Saving must not wipe keys this build does not manage.
func TestSavePreservesForeignKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IniPath(), []byte(strings.Join([]string{
		"[EuroKey]",
		"Enabled=1",
		"ToggleKey=4",   // Windows legacy numeric value
		"FutureKey=7",
		"; a comment",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	s := Defaults()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(IniPath())
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	for _, want := range []string{"ToggleKey=4", "FutureKey=7", "; a comment"} {
		if !strings.Contains(raw, want) {
			t.Errorf("Save dropped %q:\n%s", want, raw)
		}
	}
	if !strings.Contains(raw, "Language=") {
		t.Errorf("managed keys missing:\n%s", raw)
	}
}

func TestToggleKeysFromIni(t *testing.T) {
	if got := toggleKeysFromIni(map[string]string{}); len(got) != 1 || got[0] != "ctrl+shift" {
		t.Fatalf("default toggles = %v", got)
	}
	if got := toggleKeysFromIni(map[string]string{"ToggleKeyNames": ""}); len(got) != 0 {
		t.Fatalf("explicit empty must disable toggling, got %v", got)
	}
	if got := toggleKeysFromIni(map[string]string{"ToggleKey": "4"}); len(got) != 1 || got[0] != "ctrl+shift" {
		t.Fatalf("legacy numeric ToggleKey must fall back to defaults, got %v", got)
	}
}

var _ = filepath.Join
