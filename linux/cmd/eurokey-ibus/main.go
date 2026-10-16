// eurokey-ibus is the IBus engine daemon for EuroKey. ibus-daemon spawns it
// via the component XML (exec line); --standalone registers the component on a
// running daemon for development.
//
// Key handling mirrors the Windows shell: printable chars go through the
// engine, navigation/modifier chords reset it, Backspace goes through
// engine.Backspace. Output reaches the app as preedit/commit text — see
// internal/session for why committed text is never edited.
package main

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	ibus "github.com/BambooEngine/goibus"
	"github.com/godbus/dbus/v5"

	"eurokey-linux/config"
	"eurokey-linux/internal/session"
	"eurokey/engine"
)

// IBus key state masks (same values as X11/GDK modifiers).
const (
	ibusShiftMask   = 1 << 0
	ibusLockMask    = 1 << 1 // Caps Lock
	ibusControlMask = 1 << 2
	ibusMod1Mask    = 1 << 3 // Alt
	ibusMod4Mask    = 1 << 6 // Super
	ibusReleaseMask = 1 << 30
)

// IBus keyvals are X11 keysyms; the ones we need:
const (
	keyBackSpace  = 0xFF08
	keyTab        = 0xFF09
	keyISOLeftTab = 0xFE20
	keyReturn     = 0xFF0D
	keyEscape     = 0xFF1B

	keyHome     = 0xFF50
	keyLeft     = 0xFF51
	keyUp       = 0xFF52
	keyRight    = 0xFF53
	keyDown     = 0xFF54
	keyPageUp   = 0xFF55
	keyPageDown = 0xFF56
	keyEnd      = 0xFF57
	keyInsert   = 0xFF63
	keyMenu     = 0xFF67
	keyKPEnter  = 0xFF8D
	keyKPDelete = 0xFF9F
	keyKPBegin  = 0xFF9D
	keyPrint    = 0xFF61

	keyScrollLock = 0xFF14
	keyPause      = 0xFF13
	keyNumLock    = 0xFF7F
	keyDelete     = 0xFFFF

	keyF1  = 0xFFBE
	keyF35 = 0xFFE0

	keyShiftL   = 0xFFE1
	keyShiftR   = 0xFFE2
	keyControlL = 0xFFE3
	keyControlR = 0xFFE4
	keyCapsLock = 0xFFE5
	keyMetaL    = 0xFFE7
	keyMetaR    = 0xFFE8
	keyAltL     = 0xFFE9
	keyAltR     = 0xFFEA
	keySuperL   = 0xFFEB
	keySuperR   = 0xFFEC

	keyUnicodeBase = 0x01000000 // direct-unicode keysyms: 0x01000000|codepoint
)

func isResetKey(k uint32) bool {
	switch k {
	case keyReturn, keyKPEnter, keyTab, keyISOLeftTab, keyEscape,
		keyLeft, keyRight, keyUp, keyDown, keyHome, keyEnd,
		keyPageUp, keyPageDown, keyDelete, keyKPDelete, keyInsert,
		keyMenu, keyPrint, keyPause, keyNumLock, keyScrollLock, keyCapsLock,
		keyKPBegin:
		return true
	}
	return k >= keyF1 && k <= keyF35
}

func isCtrlOrShift(k uint32) bool {
	return k == keyControlL || k == keyControlR || k == keyShiftL || k == keyShiftR
}

// keyvalToChar maps an IBus keyval to the char the engine expects.
// Latin-1 keysyms 0x20–0xFF are already Unicode codepoints; the 0x01000000+
// range encodes arbitrary Unicode directly. Numpad keys map to ASCII.
func keyvalToChar(k uint32) (uint32, bool) {
	if k >= 0x20 && k <= 0xFF {
		return k, true
	}
	if k >= keyUnicodeBase && k <= 0x0110FFFF {
		return k & 0xFFFFFF, true
	}
	switch k {
	case 0xFF80: // KP_Space
		return ' ', true
	case 0xFFAA: // KP_Multiply
		return '*', true
	case 0xFFAB: // KP_Add
		return '+', true
	case 0xFFAC: // KP_Separator
		return ',', true
	case 0xFFAD: // KP_Subtract
		return '-', true
	case 0xFFAE: // KP_Decimal
		return '.', true
	case 0xFFAF: // KP_Divide
		return '/', true
	}
	if k >= 0xFFB0 && k <= 0xFFB9 { // KP_0..KP_9
		return '0' + (k - 0xFFB0), true
	}
	return 0, false
}

func u16ToString(units []uint16) string {
	return string(utf16.Decode(units))
}

// -----------------------------------------------------------------------

var engineCounter uint64

type eurokeyEngine struct {
	ibus.Engine
	eng  *engine.Engine
	sess *session.Session

	// mu guards the fields below and the engine/session state: IBus delivers
	// D-Bus method calls on separate goroutines.
	mu        sync.Mutex
	cfg       config.Settings
	enabled   bool
	ctrlHeld  bool
	shiftHeld bool

	// clean-chord tracking for the ctrl+shift toggle (fires on release)
	chordArmed bool
	chordClean bool
	modDownAt  time.Time
	modUpAt    time.Time
}

func newEuroKeyEngine(conn *dbus.Conn, path dbus.ObjectPath) *eurokeyEngine {
	e := &eurokeyEngine{Engine: ibus.BaseEngine(conn, path)}
	e.reloadConfig()
	return e
}

// reloadConfig re-reads eurokey.ini + macro.txt. Called on Enable so edits
// made outside the IME take effect when it activates.
func (e *eurokeyEngine) reloadConfig() {
	if e.eng == nil {
		e.eng = engine.New()
	}
	cfg, _ := config.Load()
	e.cfg = cfg

	opts := cfg.EngineOptions()
	e.eng.SetOptions(opts)
	e.eng.SetLanguage(cfg.Language)
	e.eng.LoadMacro(config.MacroPath())

	e.enabled = cfg.Enabled
	if e.sess == nil {
		e.sess = session.New(e.eng, cfg.Enabled)
	} else {
		e.apply(e.sess.SetEnabled(cfg.Enabled))
	}
	e.sess.AutoCap = cfg.AutoCap
}

func (e *eurokeyEngine) hasToggle(name string) bool {
	for _, k := range e.cfg.ToggleKeys {
		if k == name {
			return true
		}
	}
	return false
}

// apply turns a session.Action into IBus signals and reports whether the key
// event was consumed.
func (e *eurokeyEngine) apply(a session.Action) bool {
	if len(a.Commit) > 0 || len(a.Preedit) == 0 {
		e.UpdatePreeditText(ibus.NewText(""), 0, false)
	}
	if len(a.Commit) > 0 {
		e.CommitText(ibus.NewText(u16ToString(a.Commit)))
	}
	if len(a.Preedit) > 0 {
		t := ibus.NewText(u16ToString(a.Preedit))
		n := uint32(utf8.RuneCountInString(t.Text))
		t.AppendAttr(ibus.IBUS_ATTR_TYPE_UNDERLINE, ibus.IBUS_ATTR_UNDERLINE_SINGLE, 0, n)
		e.UpdatePreeditText(t, n, true)
	}
	return !a.Forward
}

func (e *eurokeyEngine) setEnabled(v bool) {
	e.enabled = v
	e.cfg.Enabled = v
	e.cfg.Save()
	e.apply(e.sess.SetEnabled(v))
	e.RegisterProperties(e.props())
}

func (e *eurokeyEngine) toggleEnabled() { e.setEnabled(!e.enabled) }

func (e *eurokeyEngine) setLanguage(lang int) {
	e.cfg.Language = lang
	e.cfg.Save()
	e.eng.SetLanguage(lang)
	e.apply(e.sess.Reset())
	e.RegisterProperties(e.props())
}

func (e *eurokeyEngine) onModDown(keyval uint32) {
	if !e.ctrlHeld && !e.shiftHeld {
		e.modDownAt = time.Now()
		e.chordClean = true
		e.chordArmed = false
	}
	switch keyval {
	case keyControlL, keyControlR:
		e.ctrlHeld = true
	case keyShiftL, keyShiftR:
		e.shiftHeld = true
	}
	if e.hasToggle("ctrl+shift") && e.ctrlHeld && e.shiftHeld &&
		!e.chordArmed && e.chordClean &&
		time.Since(e.modDownAt) < 800*time.Millisecond {
		e.chordArmed = true
	}
}

func (e *eurokeyEngine) onModRelease(keyval uint32) {
	if !isCtrlOrShift(keyval) {
		return
	}
	wasCtrl := keyval == keyControlL || keyval == keyControlR
	if wasCtrl {
		e.ctrlHeld = false
	} else {
		e.shiftHeld = false
	}
	if !e.chordArmed {
		return
	}
	if e.ctrlHeld || e.shiftHeld {
		e.modUpAt = time.Now()
		return
	}
	e.chordArmed = false
	if e.chordClean && time.Since(e.modUpAt) < 700*time.Millisecond {
		e.toggleEnabled()
	}
}

// ------------------------------------------------------------------
// org.freedesktop.IBus.Engine interface
// ------------------------------------------------------------------

func (e *eurokeyEngine) ProcessKeyEvent(keyval uint32, keycode uint32, state uint32) (bool, *dbus.Error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if state&ibusReleaseMask != 0 {
		e.onModRelease(keyval)
		return false, nil
	}

	// toggle hotkeys checked before the modifier gates below
	if e.hasToggle("alt+z") && (keyval == 'z' || keyval == 'Z') &&
		state&ibusMod1Mask != 0 && state&ibusControlMask == 0 {
		e.toggleEnabled()
		return true, nil
	}
	if e.hasToggle("ctrl+space") && keyval == ' ' && state&ibusControlMask != 0 {
		e.toggleEnabled()
		return true, nil
	}

	if isCtrlOrShift(keyval) {
		e.onModDown(keyval)
		return false, nil
	}
	switch keyval {
	case keyAltL, keyAltR, keyMetaL, keyMetaR, keySuperL, keySuperR:
		return false, nil
	}
	if e.ctrlHeld || e.shiftHeld {
		e.chordClean = false
	}

	// Ctrl/Alt/Super shortcuts bypass the engine and invalidate the word.
	if state&(ibusControlMask|ibusMod1Mask|ibusMod4Mask) != 0 {
		e.apply(e.sess.Reset())
		return false, nil
	}

	if isResetKey(keyval) {
		if keyval == keyReturn || keyval == keyKPEnter {
			e.apply(e.sess.EnterPressed())
		} else {
			e.apply(e.sess.Reset())
		}
		return false, nil
	}

	if keyval == keyBackSpace {
		return e.apply(e.sess.Backspace()), nil
	}

	ch, ok := keyvalToChar(keyval)
	if !ok {
		// Keysym we can't map: commit the pending word and let the app handle
		// the key itself.
		a := e.sess.Reset()
		a.Forward = true
		return e.apply(a), nil
	}
	return e.apply(e.sess.Feed(ch)), nil
}

func (e *eurokeyEngine) FocusIn() *dbus.Error { return nil }

func (e *eurokeyEngine) FocusOut() *dbus.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.apply(e.sess.Reset())
	return nil
}

func (e *eurokeyEngine) Reset() *dbus.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.apply(e.sess.Reset())
	return nil
}

func (e *eurokeyEngine) Enable() *dbus.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reloadConfig()
	e.RegisterProperties(e.props())
	return nil
}

func (e *eurokeyEngine) Disable() *dbus.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.apply(e.sess.Reset())
	return nil
}

func (e *eurokeyEngine) SetCapabilities(cap uint32) *dbus.Error { return nil }
func (e *eurokeyEngine) SetCursorLocation(x, y, w, h int32) *dbus.Error {
	return nil
}
func (e *eurokeyEngine) SetSurroundingText(text dbus.Variant, cursorIndex, anchorPos uint32) *dbus.Error {
	return nil
}
func (e *eurokeyEngine) PageUp() *dbus.Error             { return nil }
func (e *eurokeyEngine) PageDown() *dbus.Error           { return nil }
func (e *eurokeyEngine) CursorUp() *dbus.Error           { return nil }
func (e *eurokeyEngine) CursorDown() *dbus.Error         { return nil }
func (e *eurokeyEngine) PropertyShow(string) *dbus.Error { return nil }
func (e *eurokeyEngine) PropertyHide(string) *dbus.Error { return nil }
func (e *eurokeyEngine) CandidateClicked(index, button, state uint32) *dbus.Error {
	return nil
}

func (e *eurokeyEngine) PropertyActivate(propName string, propState uint32) *dbus.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch propName {
	case "enabled":
		if (propState == ibus.PROP_STATE_CHECKED) != e.enabled {
			e.toggleEnabled()
		}
	case "lang_off":
		e.setLanguage(engine.LangOff)
	case "lang_de":
		e.setLanguage(engine.LangDE)
	case "lang_fr":
		e.setLanguage(engine.LangFR)
	case "lang_es":
		e.setLanguage(engine.LangES)
	}
	return nil
}

func (e *eurokeyEngine) props() *ibus.PropList {
	enabledState := uint32(ibus.PROP_STATE_UNCHECKED)
	if e.enabled {
		enabledState = ibus.PROP_STATE_CHECKED
	}
	radio := func(key, label string, active bool) *ibus.Property {
		st := uint32(ibus.PROP_STATE_UNCHECKED)
		if active {
			st = ibus.PROP_STATE_CHECKED
		}
		return ibus.NewProperty(key, ibus.PROP_TYPE_RADIO, label, "", label, true, true, st)
	}
	return ibus.NewPropList(
		ibus.NewProperty("enabled", ibus.PROP_TYPE_TOGGLE,
			"Enabled", "", "Turn dead-key composition on/off", true, true, enabledState),
		ibus.NewPropertyWithChild("language", ibus.PROP_TYPE_MENU,
			"Language", "", "Compose language", true, true, 0, *ibus.NewPropList(
				radio("lang_off", "Off", e.cfg.Language == engine.LangOff),
				radio("lang_de", "Deutsch", e.cfg.Language == engine.LangDE),
				radio("lang_fr", "Français", e.cfg.Language == engine.LangFR),
				radio("lang_es", "Español", e.cfg.Language == engine.LangES),
			)),
	)
}

// -----------------------------------------------------------------------

var (
	standalone = flag.Bool("standalone", false, "register component on the running ibus-daemon (dev mode)")
	execPath   = flag.String("exec", "", "exec path to embed in the registered component (standalone mode)")
)

func component() *ibus.Component {
	self, err := os.Executable()
	if err != nil || *execPath != "" {
		self = *execPath
	}
	icon := filepath.Join(filepath.Dir(self), "logo.jpg")
	c := ibus.NewComponent(
		"org.freedesktop.IBus.EuroKey",
		"Dead-key accent keyboard (German, French, Spanish)",
		"0.1.0", "GPL-3.0", "EuroKey", "", self+" --ibus", "",
	)
	ed := ibus.FullEngineDesc(
		"eurokey", "EuroKey (Deutsch/Français/Español)",
		"Dead-key / compose input for German, French and Spanish",
		"de", "GPL-3.0", "EuroKey", icon, "us",
		0, "", "DE", "",
		"", "", "0.1.0", "", 
	)
	c.AddEngine(ed)
	return c
}

func main() {
	flag.Parse()

	bus, err := ibus.NewBusE()
	if err != nil {
		os.Exit(1)
	}
	defer bus.Close()

	if err := bus.RequestNameChecked("org.freedesktop.IBus.EuroKey", 0); err != nil {
		os.Exit(1)
	}

	if *standalone {
		if call := bus.RegisterComponent(component()); call.Err != nil {
			os.Exit(1)
		}
	}

	conn := bus.GetDbusConn()
	ibus.NewFactory(conn, func(conn *dbus.Conn, engineName string) dbus.ObjectPath {
		n := atomic.AddUint64(&engineCounter, 1)
		path := dbus.ObjectPath("/org/freedesktop/IBus/Engine/" + strconv.FormatUint(n, 10))
		ibus.PublishEngine(conn, path, newEuroKeyEngine(conn, path))
		return path
	})

	<-bus.Done()
}
