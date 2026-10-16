package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupAppModeTest(t *testing.T) {
	t.Helper()
	oldConfig, oldGlobal := cfg, gEnabled
	oldRemembered, oldPerApp := gRememberedAppOn, gPerAppOn
	oldEngine, oldAutoCap := gEngine, gAutoCapNext
	oldIM := gLang
	oldKeyHwnd, oldLastIM := gLastKeyHwnd, gLastLang
	oldClip, oldSafeDel, oldCaret := gForceClip, gSafeDel, gCaretAtEnd
	oldCtrl, oldAlt, oldShift, oldWin, oldHotkey := gCtrlHeld, gAltHeld, gShiftHeld, gWinHeld, gHotkeyDown
	oldHwnd, oldSpec, oldExe := lastFgHwnd, lastFgSpec, lastFgExe
	oldInputApp := gLastInputApp
	t.Cleanup(func() {
		cfg, gEnabled = oldConfig, oldGlobal
		gRememberedAppOn, gPerAppOn = oldRemembered, oldPerApp
		gEngine, gAutoCapNext = oldEngine, oldAutoCap
		gLang = oldIM
		gLastKeyHwnd, gLastLang = oldKeyHwnd, oldLastIM
		gForceClip, gSafeDel, gCaretAtEnd = oldClip, oldSafeDel, oldCaret
		gCtrlHeld, gAltHeld, gShiftHeld, gWinHeld, gHotkeyDown = oldCtrl, oldAlt, oldShift, oldWin, oldHotkey
		lastFgHwnd, lastFgSpec, lastFgExe = oldHwnd, oldSpec, oldExe
		gLastInputApp = oldInputApp
	})
	gEngine, _ = newEngine()
	gLang = LangDE
	gEngine.SetLanguage(LangDE)
	cfg = config{RememberAppMode: true}
	gEnabled = true
	gRememberedAppOn = map[string]bool{}
	gPerAppOn = map[string]bool{}
	gLastInputApp = inputAppContext{}
	gCtrlHeld, gAltHeld, gShiftHeld, gWinHeld, gHotkeyDown = false, false, false, false, false
}

func focusAppForTest(exe string, spec appSpec) {
	// Exercise real callers without moving focus on the user's desktop.
	lastFgHwnd, _, _ = pGetForegroundWindow.Call()
	lastFgSpec, lastFgExe = spec, exe
	gLastInputApp = inputAppContext{spec, exe, lastFgHwnd}
}

func TestRememberAppModeSwitchAndRestart(t *testing.T) {
	setupAppModeTest(t)
	normal := appSpec{lang: -1}
	focusAppForTest("game.exe", normal)
	if !flipEnabled() || effectiveEnabled() || !gEnabled {
		t.Fatal("disabling the game must leave the global default enabled")
	}
	focusAppForTest("notepad.exe", normal)
	if !effectiveEnabled() {
		t.Fatal("another app must still type Vietnamese")
	}
	focusAppForTest("game.exe", normal)
	if effectiveEnabled() {
		t.Fatal("returning to the game must restore English")
	}
	// The hook must use the same decision as the tray/HUD.
	cfg.HotkeyMods, cfg.HotkeyVk = 4, 'Z'
	for _, ch := range "aas" {
		if handleKeyDown(&kbdLLHookStruct{vkCode: uint32(strings.ToUpper(string(ch))[0])}) {
			t.Fatal("remembered English app must pass Telex letters through")
		}
	}
	p := filepath.Join(t.TempDir(), "tuigo.ini")
	gRememberedAppOn["chrome.exe"] = true
	if err := saveSettingsTo(p); err != nil {
		t.Fatal(err)
	}
	vals, err := readIni(p)
	if err != nil {
		t.Fatal(err)
	}
	gRememberedAppOn = nil
	loadRememberedAppModes(vals)
	if vals["RememberAppMode"] != "1" || effectiveEnabled() || !gRememberedAppOn["chrome.exe"] {
		t.Fatal("saved mode must survive reload")
	}
	if !flipEnabled() || !effectiveEnabled() {
		t.Fatal("turning Vietnamese back on must replace the saved English mode")
	}
	if err := saveSettingsTo(p); err != nil {
		t.Fatal(err)
	}
	vals, _ = readIni(p)
	loadRememberedAppModes(vals)
	if !effectiveEnabled() {
		t.Fatal("re-enabled Vietnamese must survive reload")
	}
}

func TestRememberAppModeQuickKeys(t *testing.T) {
	setupAppModeTest(t)
	focusAppForTest("game.exe", appSpec{lang: -1})
	cfg.HotkeyMods, cfg.HotkeyVk, cfg.FKeys = 4, 'Z', 1|2|16
	gAltHeld = true
	if !handleKeyDown(&kbdLLHookStruct{vkCode: 'Z'}) || effectiveEnabled() || !gEnabled {
		t.Fatal("Alt+Z must save English only for this app")
	}
	if !handleKeyDown(&kbdLLHookStruct{vkCode: 'Z'}) || effectiveEnabled() {
		t.Fatal("hotkey auto-repeat must not re-toggle")
	}
	gAltHeld = false
	if !handleKeyDown(&kbdLLHookStruct{vkCode: VK_F1}) || !effectiveEnabled() {
		t.Fatal("F1 must replace the remembered English choice")
	}
	if !handleKeyDown(&kbdLLHookStruct{vkCode: VK_F2}) || effectiveEnabled() {
		t.Fatal("F2 must save English again")
	}
	if !handleKeyDown(&kbdLLHookStruct{vkCode: VK_F12}) || effectiveEnabled() {
		t.Fatal("resetting the typing buffer must preserve remembered app choices")
	}
	for i := 0; i < 20; i++ {
		if !flipEnabled() || effectiveEnabled() != (i%2 == 0) || !gEnabled {
			t.Fatalf("rapid per-app toggle %d was lost or changed the global mode", i)
		}
	}
}

func TestRememberAppModeProfiles(t *testing.T) {
	setupAppModeTest(t)
	gRememberedAppOn = map[string]bool{"app.exe": true}
	gEnabled = false
	for _, tc := range []struct {
		name string
		mode int
		want bool
	}{
		{"normal", appModeNone, true},
		{"clipboard", appModeClip, true},
		{"manual", appModeManual, true},
		{"lock", appModeLock, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			focusAppForTest("app.exe", appSpec{mode: tc.mode, lang: LangFR})
			if effectiveEnabled() != tc.want {
				t.Fatal("profile and remembered mode disagree")
			}
			if tc.mode == appModeLock && flipEnabled() {
				t.Fatal("remembering must not bypass a locked profile")
			}
		})
	}
	// A manually excluded app can be enabled and keep that choice on return.
	gRememberedAppOn = map[string]bool{}
	focusAppForTest("manual.exe", appSpec{mode: appModeManual, lang: -1})
	if effectiveEnabled() || !flipEnabled() || !effectiveEnabled() || gEnabled {
		t.Fatal("manual exclusion must toggle per-app without changing the global default")
	}
}

func TestRememberAppModeDisabledAndUnknown(t *testing.T) {
	setupAppModeTest(t)
	gRememberedAppOn["game.exe"] = false
	focusAppForTest("game.exe", appSpec{lang: -1})
	cfg.RememberAppMode = false
	if !effectiveEnabled() || !flipEnabled() || gEnabled || gRememberedAppOn["game.exe"] {
		t.Fatal("disabled feature must keep the global toggle and preserve saved choices")
	}
	cfg.RememberAppMode = true
	gEnabled = true
	if effectiveEnabled() {
		t.Fatal("re-enabling must restore the saved choice")
	}
	focusAppForTest("", appSpec{lang: -1})
	if !flipEnabled() || gEnabled {
		t.Fatal("unknown process must fall back to the global toggle")
	}
	if _, ok := gRememberedAppOn[""]; ok {
		t.Fatal("unknown processes must never share an empty app identity")
	}
}

func TestLoadRememberedAppModesValidation(t *testing.T) {
	setupAppModeTest(t)
	loadRememberedAppModes(map[string]string{
		"AppMode.NotePad.EXE": "0", "AppMode.browser.exe": "1",
		"AppMode.": "0", "AppMode.bad.exe": "invalid",
		"AppMode.dir/app.exe": "1", "AppMode.bad\nname.exe": "0",
		"Unrelated": "1",
	})
	if len(gRememberedAppOn) != 2 || gRememberedAppOn["notepad.exe"] || !gRememberedAppOn["browser.exe"] {
		t.Fatalf("unexpected restored choices: %v", gRememberedAppOn)
	}
	loadRememberedAppModes(map[string]string{})
	if len(gRememberedAppOn) != 0 {
		t.Fatal("old settings without saved apps must start with an empty map")
	}
}

func TestSaveRememberedAppModesFailure(t *testing.T) {
	setupAppModeTest(t)
	p := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveSettingsTo(p); err == nil {
		t.Fatal("a failed rename must report the persistence error")
	}
}

func TestRememberModeUnseenAppDoesNotInheritGlobalEnglish(t *testing.T) {
	setupAppModeTest(t)
	gEnabled = false
	if !enabledForApp(appSpec{lang: -1}, "notepad.exe") {
		t.Fatal("an unseen app must default to Vietnamese when remembering is on")
	}
	if enabledForApp(appSpec{mode: appModeManual, lang: -1}, "game.exe") {
		t.Fatal("an explicitly excluded app must still default to English")
	}
	gRememberedAppOn["notepad.exe"] = false
	if enabledForApp(appSpec{lang: -1}, "notepad.exe") {
		t.Fatal("a saved English choice must override the new-app default")
	}
}

func TestEnableRememberModePreservesTypingApp(t *testing.T) {
	setupAppModeTest(t)
	cfg.RememberAppMode = false
	gEnabled = false
	focusAppForTest("chrome.exe", appSpec{lang: -1})
	setRememberAppMode(true)
	if effectiveEnabled() || gRememberedAppOn["chrome.exe"] {
		t.Fatal("enabling must preserve the typing app's existing English mode")
	}
	focusAppForTest("notepad.exe", appSpec{lang: -1})
	if !effectiveEnabled() {
		t.Fatal("other apps must default to Vietnamese instead of inheriting global English")
	}
}

func TestTrayAndSettingsKeepTypingApp(t *testing.T) {
	setupAppModeTest(t)
	chrome := inputAppContext{appSpec{lang: -1}, "chrome.exe", 100}
	notepad := inputAppContext{appSpec{lang: -1}, "notepad.exe", 200}
	resolveInputApp(chrome, "Chrome_WidgetWin_1", false)
	for _, cls := range []string{"Shell_TrayWnd", "Shell_SecondaryTrayWnd", "NotifyIconOverflowWindow", "TopLevelWindowForOverflowXamlIsland"} {
		shell := inputAppContext{appSpec{lang: -1}, "explorer.exe", 300}
		if got := resolveInputApp(shell, cls, false); got != chrome {
			t.Fatalf("%s replaced the typing app: %+v", cls, got)
		}
	}
	settings := inputAppContext{appSpec{lang: -1}, "tuigo.exe", 400}
	if got := resolveInputApp(settings, "TuiGoSettings", true); got != chrome {
		t.Fatal("opening settings must not replace the typing app")
	}
	// Capture on tray mouse-up; the user changes focus before the delayed timer.
	captured := gLastInputApp
	resolveInputApp(notepad, "Notepad", false)
	if !flipEnabledForApp(captured) || gRememberedAppOn["chrome.exe"] {
		t.Fatal("deferred tray toggle must affect the captured Chrome app")
	}
	if !enabledForApp(notepad.spec, notepad.exe) {
		t.Fatal("a focus change during the double-click interval must not toggle Notepad")
	}
	if _, ok := gRememberedAppOn["explorer.exe"]; ok {
		t.Fatal("tray click must never remember a preference for Explorer")
	}
	if _, ok := gRememberedAppOn["tuigo.exe"]; ok {
		t.Fatal("settings must never remember a preference for Tui Go")
	}
	// A real Explorer file-management window is a valid typing app.
	explorer := inputAppContext{appSpec{lang: -1}, "explorer.exe", 500}
	if got := resolveInputApp(explorer, "CabinetWClass", false); got != explorer {
		t.Fatal("ordinary Explorer windows must keep their own mode")
	}
}

func TestNormalTypingWithRememberedModes(t *testing.T) {
	setupAppModeTest(t)
	app := inputAppContext{appSpec{lang: -1}, "notepad.exe", 100}
	typeWord := func(keys string) string {
		gEngine.Reset()
		gEngine.SetEnabled(enabledForApp(app.spec, app.exe))
		var out []rune
		for _, ch := range keys {
			res := gEngine.Filter(uint32(ch))
			if !res.Consumed {
				out = append(out, ch)
				continue
			}
			out = out[:len(out)-res.Backs]
			for _, ch := range res.Out {
				out = append(out, rune(ch))
			}
		}
		return string(out)
	}
	if got := typeWord(`"a`); got != "ä" {
		t.Fatalf("German compose with remembering on: %q", got)
	}
	flipEnabledForApp(app)
	if got := typeWord(`"a`); got != `"a` {
		t.Fatalf("explicit off mode: %q", got)
	}
	flipEnabledForApp(app)
	if got := typeWord(`"a`); got != "ä" {
		t.Fatalf("German after re-enabling: %q", got)
	}
}

// Drive the production keyboard handler, while preventing SendInput from
// writing into the user's foreground window. The real engine still runs.
type typingModeProbe struct {
	Engine
	on bool
}

func (p *typingModeProbe) SetEnabled(on bool) {
	p.on = on
	p.Engine.SetEnabled(on)
}

func (p *typingModeProbe) Filter(ch uint32) Result {
	p.Engine.Filter(ch)
	return Result{}
}

func (p *typingModeProbe) Backspace() Result {
	p.Engine.Backspace()
	return Result{}
}

func TestOrdinaryTypingNeverSwitchesToEnglish(t *testing.T) {
	setupAppModeTest(t)
	focusAppForTest("normal-typing.exe", appSpec{lang: -1})
	cfg.RememberAppMode = false
	setRememberAppMode(true)
	cfg.HotkeyMods, cfg.HotkeyVk = 4, 'Z'
	probe := &typingModeProbe{Engine: gEngine, on: true}
	gEngine = probe
	// Includes every letter, Vietnamese tone keys, digits, punctuation,
	// whitespace and ordinary editing keys. None is a V/E command.
	for _, im := range []int{LangDE, LangFR, LangES} {
		oldIM := gLang
		gLang = im
		for pass := 0; pass < 50; pass++ {
			for _, vk := range []uint32{
				'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
				'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
				'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
				VK_SPACE, VK_BACK, VK_LEFT, VK_RIGHT, VK_RETURN, VK_TAB,
			} {
				handleKeyDown(&kbdLLHookStruct{vkCode: vk})
				if !probe.on || !effectiveEnabled() || !gRememberedAppOn["normal-typing.exe"] {
					t.Fatalf("method %d, pass %d, vk %d: normal typing switched V to E", im, pass, vk)
				}
			}
		}
		gLang = oldIM
	}
}
