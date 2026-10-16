package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Settings live in eurokey.ini next to the exe — the app is portable, so it
// must not leave traces in the registry. The Run key / scheduled task used by
// "start with Windows" is Windows infrastructure (only written when the user
// opts in), not app settings.

func iniPath() string {
	return filepath.Join(exeDir(), "eurokey.ini")
}

// readIni parses a minimal key=value ini (one section, ; and # comments).
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

var (
	saveWorkerOnce sync.Once
	saveCh         = make(chan string, 1)
	saveDiskMu     sync.Mutex
)

// saveSettings formats the INI snapshot on the UI thread and writes it to disk
// on a background goroutine so disk / antivirus file-rename scans never block
// the WH_KEYBOARD_LL message loop.
func saveSettings() {
	data := formatSettingsIni()
	saveWorkerOnce.Do(func() {
		go func() {
			for s := range saveCh {
				_ = writeIniAtomic(iniPath(), s)
			}
		}()
	})
	select {
	case saveCh <- data:
	default:
		select {
		case <-saveCh:
		default:
		}
		select {
		case saveCh <- data:
		default:
		}
	}
}

func saveSettingsSync() {
	_ = saveSettingsTo(iniPath())
}

func formatSettingsIni() string {
	bv := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	o := gEngine.GetOptions()
	var b strings.Builder
	b.WriteString("[EuroKey]\n")
	fmt.Fprintf(&b, "Enabled=%s\n", bv(gEnabled))
	fmt.Fprintf(&b, "Language=%d\n", gLang)
	fmt.Fprintf(&b, "MacroEnabled=%d\n", o.MacroEnabled)
	fmt.Fprintf(&b, "SkipNonUSLayout=%s\n", bv(cfg.SkipNonUSLayout))
	fmt.Fprintf(&b, "AutoCap=%s\n", bv(cfg.AutoCap))
	fmt.Fprintf(&b, "UseClipboard=%s\n", bv(cfg.UseClipboard))
	fmt.Fprintf(&b, "ShowHud=%s\n", bv(cfg.ShowHud))
	fmt.Fprintf(&b, "RememberAppMode=%s\n", bv(cfg.RememberAppMode))
	fmt.Fprintf(&b, "RunAsAdmin=%s\n", bv(cfg.RunAsAdmin))
	fmt.Fprintf(&b, "ShowOnLaunch=%s\n", bv(cfg.ShowOnLaunch))
	fmt.Fprintf(&b, "SoundOnToggle=%s\n", bv(cfg.SoundOnToggle))
	fmt.Fprintf(&b, "ToggleKey=%d\n", cfg.ToggleKey)
	fmt.Fprintf(&b, "HotkeyMods=%d\n", cfg.HotkeyMods)
	fmt.Fprintf(&b, "HotkeyVk=%d\n", cfg.HotkeyVk)
	fmt.Fprintf(&b, "FKeys=%d\n", cfg.FKeys)
	fmt.Fprintf(&b, "UseCtrlShift=%s\n", bv(cfg.UseCtrlShift))
	fmt.Fprintf(&b, "Theme=%d\n", cfg.Theme)
	fmt.Fprintf(&b, "Style=%d\n", cfg.Style)
	fmt.Fprintf(&b, "UILang=%d\n", cfg.UILang)
	fmt.Fprintf(&b, "HudX=%d\n", cfg.HudX)
	fmt.Fprintf(&b, "HudY=%d\n", cfg.HudY)
	names := make([]string, 0, len(gRememberedAppOn))
	for name := range gRememberedAppOn {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "AppMode.%s=%s\n", name, bv(gRememberedAppOn[name]))
	}
	return b.String()
}

func writeIniAtomic(path, content string) error {
	saveDiskMu.Lock()
	defer saveDiskMu.Unlock()
	// Atomic-ish: write temp then rename so a crash mid-write can't leave a
	// truncated ini that silently resets every option on next start.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func saveSettingsTo(path string) error {
	existing, _ := os.ReadFile(path)
	return writeIniAtomic(path, mergeIniPreserve(string(existing), formatSettingsIni()))
}

// mergeIniPreserve keeps lines whose keys this build does not write plus
// comments, so saving never wipes settings owned by another build or platform.
// AppMode.* is fully managed — stale entries are dropped and rewritten.
func mergeIniPreserve(existing, generated string) string {
	written := map[string]bool{}
	for _, line := range strings.Split(generated, "\n") {
		if k, _, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			written[strings.TrimSpace(k)] = true
		}
	}
	var preserved []string
	for _, line := range strings.Split(existing, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || t[0] == '[' {
			continue // section headers are cosmetic; generated writes its own
		}
		if t[0] == ';' || t[0] == '#' {
			preserved = append(preserved, line)
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok {
			key := strings.TrimSpace(k)
			if written[key] || strings.HasPrefix(key, "AppMode.") {
				continue
			}
			preserved = append(preserved, line)
		}
	}
	if len(preserved) == 0 {
		return generated
	}
	// splice preserved lines directly under the [EuroKey] header
	lines := strings.SplitN(generated, "\n", 2)
	return lines[0] + "\n" + strings.Join(preserved, "\n") + "\n" + lines[1]
}

func loadRememberedAppModes(vals map[string]string) {
	gRememberedAppOn = map[string]bool{}
	for key, value := range vals {
		name, ok := strings.CutPrefix(key, "AppMode.")
		name = strings.ToLower(strings.TrimSpace(name))
		if ok && name != "" && !strings.ContainsAny(name, "/\\=\r\n") && (value == "0" || value == "1") {
			gRememberedAppOn[name] = value == "1"
		}
	}
}

// loadSettings reads persisted config. Reports whether config already
// existed — false means first run (show the panel once for onboarding).
func loadSettings() bool {
	vals, err := readIni(iniPath())
	if err != nil {
		return false
	}
	getI := func(name string) (int, bool) {
		s, ok := vals[name]
		if !ok {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	getB := func(name string, dflt bool) bool {
		if s, ok := vals[name]; ok {
			return s == "1"
		}
		return dflt
	}
	if v, ok := getI("Enabled"); ok {
		gEnabled = v != 0
	}
	if v, ok := getI("Language"); ok {
		gLang = v
		gEngine.SetLanguage(gLang)
	}
	o := gEngine.GetOptions()
	if v, ok := getI("MacroEnabled"); ok {
		o.MacroEnabled = int32(v)
	}
	gEngine.SetOptions(o)
	cfg.SkipNonUSLayout = getB("SkipNonUSLayout", false)
	cfg.AutoCap = getB("AutoCap", false)
	cfg.UseClipboard = getB("UseClipboard", false)
	cfg.ShowHud = getB("ShowHud", false)
	cfg.RememberAppMode = getB("RememberAppMode", false)
	loadRememberedAppModes(vals)
	cfg.RunAsAdmin = getB("RunAsAdmin", false)
	cfg.ShowOnLaunch = getB("ShowOnLaunch", false)
	cfg.SoundOnToggle = getB("SoundOnToggle", false)
	if v, ok := getI("ToggleKey"); ok {
		cfg.ToggleKey = v
	}
	if v, ok := getI("HotkeyMods"); ok {
		cfg.HotkeyMods = v
	}
	if v, ok := getI("HotkeyVk"); ok {
		cfg.HotkeyVk = v
	}
	if v, ok := getI("FKeys"); ok {
		cfg.FKeys = v
	}
	// migrate legacy ToggleKey -> new hotkey model
	if cfg.HotkeyVk == 0 && cfg.HotkeyMods == 0 {
		switch cfg.ToggleKey {
		case 2: // Ctrl+Shift only
			cfg.HotkeyMods, cfg.HotkeyVk = 3, 0
		default: // Alt+Z (+ optional Ctrl+Shift)
			cfg.HotkeyMods, cfg.HotkeyVk = 4, 'Z'
		}
	}
	cfg.UseCtrlShift = getB("UseCtrlShift", cfg.ToggleKey != 1)
	if v, ok := getI("Theme"); ok {
		cfg.Theme = v
	}
	if v, ok := getI("Style"); ok {
		cfg.Style = v
	}
	if cfg.Theme >= 3 && cfg.Theme <= 6 {
		cfg.Style = cfg.Theme - 2
		cfg.Theme = 0
	}
	if cfg.Style < 0 || cfg.Style > 4 {
		cfg.Style = 0
	}
	if cfg.Theme < 0 || cfg.Theme > 2 {
		cfg.Theme = 0
	}
	if v, ok := getI("UILang"); ok && (v == 0 || v == 1) {
		cfg.UILang = v
	}
	if v, ok := getI("HudX"); ok {
		cfg.HudX = v
	}
	if v, ok := getI("HudY"); ok {
		cfg.HudY = v
	}
	return true
}
