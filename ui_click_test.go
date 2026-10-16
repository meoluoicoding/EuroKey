package main

import (
	"os"
	"testing"
)

// In-process Win32 UI automation for the EuroKey settings window.
//
// "Clicking" sends WM_COMMAND so the real settingsProc -> settingsCommand path
// runs synchronously (no message loop needed). *_test.go files are exempt from
// the no-diagnostics rule, but these tests do not log.
//
// The settings window is a real window: the tests skip when there is no
// interactive desktop session (e.g. a headless CI runner).

// uiOpenSettings opens the real settings window and skips when it cannot be
// created.
func uiOpenSettings(t *testing.T) {
	t.Helper()
	// settingsProc recovers panics internally, so a nil engine would make every
	// click silently do nothing. Ensure a real engine exists first.
	if gEngine == nil {
		e, err := newEngine()
		if err != nil {
			t.Fatal(err)
		}
		gEngine = e
		gEngine.SetLanguage(LangDE)
	}
	openSettings()
	if gSettingsHwnd == 0 {
		t.Skip("cannot create settings window (needs an interactive desktop session)")
	}
	t.Cleanup(func() {
		if gSettingsHwnd != 0 {
			pDestroyWindow.Call(gSettingsHwnd)
			gSettingsHwnd = 0
		}
	})
}

// uiSnapshotIni saves eurokey.ini and restores it afterwards, because
// settingsCommand persists on every click.
func uiSnapshotIni(t *testing.T) {
	t.Helper()
	path := iniPath()
	old, err := os.ReadFile(path)
	existed := err == nil
	t.Cleanup(func() {
		if existed {
			_ = os.WriteFile(path, old, 0o644)
		} else {
			_ = os.Remove(path)
		}
	})
}

// uiClick simulates a BUTTON click: WM_COMMAND with BN_CLICKED (code = 0).
func uiClick(t *testing.T, id int) {
	t.Helper()
	if gSettingsHwnd == 0 {
		t.Fatal("settings window not open — call uiOpenSettings first")
	}
	pSendMessage.Call(gSettingsHwnd, WM_COMMAND, uintptr(id), ctlHnd[id])
}

// TestUiClickLanguageRadios clicks each language radio and asserts the engine
// language follows the real handler.
func TestUiClickLanguageRadios(t *testing.T) {
	uiSnapshotIni(t)
	uiOpenSettings(t)
	oldLang := gLang
	t.Cleanup(func() { gLang = oldLang })

	for _, tc := range []struct {
		name string
		id   int
		want int
	}{
		{"Deutsch", cRadioDE, LangDE},
		{"Francais", cRadioFR, LangFR},
		{"Espanol", cRadioES, LangES},
		{"Off", cRadioOff, LangOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uiClick(t, tc.id)
			if gLang != tc.want {
				t.Fatalf("click %s: gLang = %d, want %d", tc.name, gLang, tc.want)
			}
		})
	}
}

// TestUiComboLanguage selects Deutsch in the language combo box.
func TestUiComboLanguage(t *testing.T) {
	uiSnapshotIni(t)
	uiOpenSettings(t)
	oldLang := gLang
	t.Cleanup(func() { gLang = oldLang })

	// langList = {Off, Deutsch, Francais, Espanol}: index 2 == Francais.
	// (Index 1 == Deutsch would pass trivially because it is the default.)
	pSendMessage.Call(ctlHnd[cIMCombo], CB_SETCURSEL, 2, 0)
	pSendMessage.Call(gSettingsHwnd, WM_COMMAND,
		uintptr(cIMCombo)|uintptr(CBN_SELCHANGE)<<16, ctlHnd[cIMCombo])
	if gLang != LangFR {
		t.Fatalf("combo select Francais: gLang = %d, want %d", gLang, LangFR)
	}
}

// TestUiMacroToggle flips the macro switch and asserts the engine option moved.
func TestUiMacroToggle(t *testing.T) {
	uiSnapshotIni(t)
	uiOpenSettings(t)
	before := gEngine.GetOptions().MacroEnabled
	uiClick(t, cMacro)
	after := gEngine.GetOptions().MacroEnabled
	if after == before {
		t.Fatalf("MacroEnabled did not change after click (before=%d)", before)
	}
}
