package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	idmToggle = iota + 1
	idmLangOff
	idmLangDE
	idmLangFR
	idmLangES
	idmMacro
	idmSettings
	idmExit
)

var (
	gIconOn, gIconOff uintptr
	gLang             = LangDE
)

// langMenuID maps a language id to its tray menu command id.
func langMenuID(lang int) uintptr {
	switch lang {
	case LangOff:
		return idmLangOff
	case LangFR:
		return idmLangFR
	case LangES:
		return idmLangES
	}
	return idmLangDE
}

// Icon handles are recreated only when theme or display scale changes.
var trayDark bool
var trayTheme = -1
var trayStyle = -1
var traySize int
var trayLastOn bool
var gTrayToggleApp inputAppContext // target captured before the deferred single click
var gTrayMenuApp inputAppContext

func refreshTrayIcons() {
	dark := isDark()
	size, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
	if size < 16 || size > 128 {
		size = 16
	}
	if gIconOn != 0 && gIconOff != 0 && trayDark == dark && trayTheme == cfg.Theme && trayStyle == cfg.Style && traySize == int(size) {
		return
	}
	p := curPalette()
	on, off := makeTrayIcon(true, int(size), p), makeTrayIcon(false, int(size), p)
	if on == 0 || off == 0 {
		pDestroyIcon.Call(on)
		pDestroyIcon.Call(off)
		return // retain the previous pair if allocation failed
	}
	oldOn, oldOff := gIconOn, gIconOff
	gIconOn, gIconOff = on, off
	trayDark, trayTheme, trayStyle, traySize = dark, cfg.Theme, cfg.Style, int(size)
	if gTrayAdded {
		trayUpdate()
	}
	if oldOn != 0 {
		pDestroyIcon.Call(oldOn)
	}
	if oldOff != 0 {
		pDestroyIcon.Call(oldOff)
	}
}

// Runs on the UI thread, including when focus changes without a keystroke.
func syncTrayState() {
	changed := effectiveEnabled() != trayLastOn
	themeChanged := trayDark != isDark() || trayTheme != cfg.Theme || trayStyle != cfg.Style
	refreshTrayIcons()
	if changed || themeChanged {
		trayUpdate()
		hudUpdate()
		if gSettingsHwnd != 0 && (themeChanged || gActiveTab == 0) {
			pInvalidateRect.Call(gSettingsHwnd, 0, 1)
		}
	}
}

func notifyIcon(flags uint32) *notifyIconData {
	nid := &notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             gHwnd,
		uID:              1,
		uFlags:           flags,
		uCallbackMessage: WM_TRAYICON,
	}
	if effectiveEnabled() {
		nid.hIcon = gIconOn
		copy(nid.szTip[:], windows.StringToUTF16(tr(LangName(gLang)+" đang bật", LangName(gLang)+": ON")))
	} else {
		nid.hIcon = gIconOff
		copy(nid.szTip[:], windows.StringToUTF16(tr(LangName(gLang)+" đang tắt", LangName(gLang)+": OFF")))
	}
	return nid
}

var gTrayAdded bool

func trayAdd() {
	r, _, _ := pShellNotifyIcon.Call(NIM_ADD,
		uintptr(unsafe.Pointer(notifyIcon(NIF_ICON|NIF_TIP|NIF_MESSAGE))))
	// At logon Explorer may not be ready yet — NIM_ADD can fail; the
	// WM_TIMER watchdog retries until it lands. TaskbarCreated broadcast
	// re-adds after every Explorer (re)start too.
	gTrayAdded = r != 0
	trayLastOn = effectiveEnabled()
}

func trayUpdate() {
	r, _, _ := pShellNotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(notifyIcon(NIF_ICON|NIF_TIP))))
	gTrayAdded = r != 0
	trayLastOn = effectiveEnabled()
}

func trayDelete() {
	pShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(notifyIcon(0))))
	gTrayAdded = false
	pDestroyIcon.Call(gIconOn)
	pDestroyIcon.Call(gIconOff)
	gIconOn, gIconOff = 0, 0
}

func appendMenu(menu uintptr, flags uint32, id uintptr, text string) {
	pAppendMenu.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(utf16ptr(text))))
}

func checked(b bool) uint32 {
	if b {
		return MF_CHECKED
	}
	return 0
}

func showTrayMenu() {
	gTrayMenuApp = currentInputApp()
	menu, _, _ := pCreatePopupMenu.Call()
	langMenu, _, _ := pCreatePopupMenu.Call()
	o := gEngine.GetOptions()

	appendMenu(menu, MF_STRING|checked(effectiveEnabled()), idmToggle, tr("Bật tắt\tAlt+Z", "Enabled\tAlt+Z"))
	for _, lang := range LangIDs {
		appendMenu(langMenu, MF_STRING|checked(gLang == lang), langMenuID(lang), LangName(lang))
	}
	appendMenu(menu, MF_STRING|MF_POPUP, langMenu, tr("Ngôn ngữ", "Language"))
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING|checked(o.MacroEnabled != 0), idmMacro, tr("Gõ tắt", "Macros"))
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING, idmSettings, tr("Cài đặt…", "Settings…"))
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING, idmExit, tr("Thoát", "Exit"))

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWnd.Call(gHwnd)
	pTrackPopupMenu.Call(menu, TPM_BOTTOMALIGN|TPM_LEFTALIGN,
		uintptr(pt.x), uintptr(pt.y), 0, gHwnd, 0)
	pDestroyMenu.Call(menu)
}

// flipEnabled flips the effective enabled state. Cheap — pure memory
// ops plus the cached foreground-app lookup — so it can run inside the
// keyboard hook and the very next keystroke already sees the new state.
// Returns false when the foreground app is locked (toggle ignored).
func flipEnabled() bool {
	app := currentInputApp()
	return flipEnabledForApp(app)
}

func flipEnabledForApp(app inputAppContext) bool {
	spec, exe := app.spec, app.exe
	if spec.mode == appModeLock {
		return false
	}
	if cfg.RememberAppMode && exe != "" {
		on := !enabledForApp(spec, exe)
		gRememberedAppOn[exe] = on
		gEngine.Reset()
		gAutoCapNext = false
		gEngine.SetEnabled(on)
		return true
	}
	if spec.mode == appModeManual { // per-app toggle: does not touch global state
		gPerAppOn[exe] = !gPerAppOn[exe]
		return true
	}
	gEnabled = !gEnabled
	// The global toggle is the master switch: drop every per-app override
	// so an excluded app toggled on earlier returns to exclusion — without
	// this it stays un-excluded for the whole session with no way back.
	gPerAppOn = map[string]bool{}
	gEngine.SetEnabled(gEnabled)
	return true
}

// toggleEffects is the heavyweight half of a toggle — beep, tray
// RPC, HUD update, async ini save. Posted onto the message loop so the hook
// never pays for it.
func toggleEffects(_ string) {
	spec, _, _ := foregroundApp()
	if cfg.SoundOnToggle {
		pMessageBeep.Call(0x00000040) // MB_ICONASTERISK
	}
	trayUpdate()
	hudUpdate()
	if gSettingsHwnd != 0 && gActiveTab == 0 {
		pInvalidateRect.Call(gSettingsHwnd, 0, 1) // refresh mode indicator on Overview tab
	}
	if cfg.RememberAppMode || spec.mode != appModeManual {
		saveSettings()
	}
}

func toggleEnabled(src string) {
	if flipEnabled() {
		toggleEffects(src)
	}
}

func toggleEnabledForApp(src string, app inputAppContext) {
	if flipEnabledForApp(app) {
		toggleEffects(src)
	}
}

// toggleMacro flips macro expansion (F9 quick action).
func toggleMacro() {
	o := gEngine.GetOptions()
	o.MacroEnabled = 1 - o.MacroEnabled
	gEngine.SetOptions(o)
	if ctlHnd[cMacro] != 0 { // keep settings UI in sync if open
		setCheck(cMacro, o.MacroEnabled != 0)
	}
	saveSettings()
	if cfg.SoundOnToggle {
		pMessageBeep.Call(0x00000040)
	}
}

func setLanguage(lang int) {
	gLang = lang
	gEngine.SetLanguage(lang)
	gLastLang = lang
	hudUpdate()
	repaintRadioGroup() // covers tray-menu switches while settings is open
	saveSettings()
}

func toggleOption(id uintptr) {
	if id != idmMacro {
		return
	}
	o := gEngine.GetOptions()
	o.MacroEnabled ^= 1
	gEngine.SetOptions(o)
	saveSettings()
}
