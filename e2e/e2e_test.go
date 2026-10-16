// EuroKey end-to-end typing test.
//
// rod drives the browser (opens a page, reads the typed value, can screenshot),
// but it CANNOT trigger EuroKey: CDP key events never reach the Windows
// low-level keyboard hook. So the actual typing is injected at the OS level
// with SendInput, tagged with dwExtraInfo = EUROKEY_TEST_MAGIC, which the app's
// hook accepts as real input (see features.go). rod then reads the resulting
// text out of the page.
//
// Requirements:
//   - Windows desktop session (the browser window must be focusable).
//   - No other EuroKey instance running: the app is single-instance, and the
//     magic is an environment variable that can only be set at launch.
//   - A Chrome/Edge install, or rod will download one.
//
// Run:
//
//	cd e2e
//	set EUROKEY_E2E=1
//	go test -run TestEuroKeyTyping -v
//
// Env:
//
//	EUROKEY_E2E=1      opt in (launches a GUI app + browser)
//	EUROKEY_APP        path to eurokey.exe (default ..\eurokey.exe)
//	EUROKEY_E2E_LANG   de|fr|es to force; default is read from eurokey.ini
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"golang.org/x/sys/windows"
)

// ---- Win32 SendInput with dwExtraInfo ------------------------------------

const (
	inputKeyboard  = 1
	keyEventFKeyUp = 0x0002
	vkShift        = 0x10
	vkOem7         = 0xDE // ' "
	vkOem3         = 0xC0 // ` ~
	vkOemComma     = 0xBC // ,
	vkOem2         = 0xBF // / ?
	swRestore      = 9
)

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// INPUT on x64: type(4) + pad(4) + 32-byte union = 40 bytes.
type input struct {
	Type uint32
	_    uint32
	Ki   keybdInput
	_    [8]byte
}

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	pSendInput           = user32.NewProc("SendInput")
	pEnumWindows         = user32.NewProc("EnumWindows")
	pGetWindowTextW      = user32.NewProc("GetWindowTextW")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
)

var magic uintptr

func inputFor(vk uint16, flags uint32) input {
	var in input
	in.Type = inputKeyboard
	in.Ki.wVk = vk
	in.Ki.dwFlags = flags
	in.Ki.dwExtraInfo = magic
	return in
}

func send(in []input) {
	if len(in) == 0 {
		return
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(input{}))
}

// press sends one key with the magic so the hook processes it as real input.
func press(vk uint16, shift bool) {
	var in []input
	if shift {
		in = append(in, inputFor(vkShift, 0))
	}
	in = append(in, inputFor(vk, 0), inputFor(vk, keyEventFKeyUp))
	if shift {
		in = append(in, inputFor(vkShift, keyEventFKeyUp))
	}
	send(in)
}

// runeToKey maps a US-layout character to the virtual key + shift state the
// hook's vkToChar (ToUnicodeEx) expects.
func runeToKey(r rune) (uint16, bool, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return uint16(r - 'a' + 'A'), false, true
	case r >= 'A' && r <= 'Z':
		return uint16(r), true, true
	}
	switch r {
	case '"':
		return vkOem7, true, true
	case '\'':
		return vkOem7, false, true
	case '`':
		return vkOem3, false, true
	case '~':
		return vkOem3, true, true
	case ',':
		return vkOemComma, false, true
	case '?':
		return vkOem2, true, true
	case '!':
		return '1', true, true
	case '^':
		return '6', true, true
	case ' ':
		return 0x20, false, true
	}
	return 0, false, false
}

func typeString(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		vk, shift, ok := runeToKey(r)
		if !ok {
			t.Fatalf("no key mapping for %q", r)
		}
		press(vk, shift)
		time.Sleep(12 * time.Millisecond)
	}
	time.Sleep(80 * time.Millisecond)
}

// ---- window focus --------------------------------------------------------

func findWindowByTitle(marker string) uintptr {
	var found uintptr
	cb := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var buf [512]uint16
		n, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 && strings.Contains(windows.UTF16ToString(buf[:n]), marker) {
			found = hwnd
			return 0 // stop
		}
		return 1 // continue
	})
	pEnumWindows.Call(cb, 0)
	return found
}

func focusWindow(t *testing.T, hwnd uintptr) {
	t.Helper()
	pShowWindow.Call(hwnd, swRestore)
	pSetForegroundWindow.Call(hwnd)
	time.Sleep(150 * time.Millisecond)
	fg, _, _ := pGetForegroundWindow.Call()
	if fg != hwnd {
		t.Skipf("could not bring the browser window to the foreground (fg=%x, want %x) — "+
			"click the window once and rerun", fg, hwnd)
	}
}

// ---- app + language ------------------------------------------------------

func readIniLanguage(dir string) int {
	b, err := os.ReadFile(filepath.Join(dir, "eurokey.ini"))
	if err != nil {
		return 1 // default language is Deutsch
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "Language" {
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil {
				return n
			}
		}
	}
	return 1
}

// cases[language] = typed sequence -> expected page value.
var cases = map[int][]struct {
	keys string
	want string
}{
	1: { // Deutsch
		{`"a`, "ä"},
		{`"o`, "ö"},
		{`"u`, "ü"},
		{`"s`, "ß"},
		{`"S`, "ẞ"},
	},
	2: { // Français
		{`'e`, "é"},
		{`,c`, "ç"},
		{`^e`, "ê"},
	},
	3: { // Español
		{`~n`, "ñ"},
		{`'a`, "á"},
		{`??`, "¿"},
		{`!!`, "¡"},
	},
}

var langNames = map[int]string{1: "Deutsch", 2: "Français", 3: "Español"}

func TestEuroKeyTyping(t *testing.T) {
	if os.Getenv("EUROKEY_E2E") == "" {
		t.Skip("set EUROKEY_E2E=1 to run the end-to-end typing test")
	}

	app := os.Getenv("EUROKEY_APP")
	if app == "" {
		app = filepath.Join("..", "eurokey.exe")
	}
	abs, err := filepath.Abs(app)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("app not found: %s (build it first)", abs)
	}
	dir := filepath.Dir(abs)

	lang := 0
	if s := os.Getenv("EUROKEY_E2E_LANG"); s != "" {
		switch s {
		case "de":
			lang = 1
		case "fr":
			lang = 2
		case "es":
			lang = 3
		}
	} else {
		lang = readIniLanguage(dir)
	}
	if _, ok := cases[lang]; !ok {
		t.Skip("EuroKey language is Off or unknown; select Deutsch/Français/Español in the app first")
	}

	magic = 0xE0E0E0
	cmd := exec.Command(abs)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("EUROKEY_TEST_MAGIC=%d", magic))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-exited:
		t.Skip("another EuroKey instance is already running (single-instance). " +
			"Close it (tray -> Exit) and rerun so the test can set EUROKEY_TEST_MAGIC.")
	case <-time.After(2 * time.Second):
	}
	defer func() {
		_ = cmd.Process.Kill()
		<-exited
	}()

	l := launcher.New().Headless(false).Set("no-first-run").Set("no-default-browser-check")
	if bin := findBrowser(); bin != "" {
		l = l.Bin(bin)
	}
	control, err := l.Launch()
	if err != nil {
		t.Skipf("cannot launch a browser (install Chrome/Edge): %v", err)
	}
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.MustClose()
	page := browser.MustPage("data:text/html,<title>EUROKEY_E2E</title><textarea id=t style='width:900px;height:300px'></textarea>")
	page.MustWaitLoad()

	hwnd := findWindowByTitle("EUROKEY_E2E")
	if hwnd == 0 {
		t.Skip("could not find the browser window by title")
	}
	focusWindow(t, hwnd)
	page.MustEval(`() => document.querySelector('#t').focus()`)

	t.Logf("language: %s — typing %d sequences", langNames[lang], len(cases[lang]))
	for _, c := range cases[lang] {
		page.MustEval(`() => { const t = document.querySelector('#t'); t.value = ''; t.focus() }`)
		typeString(t, c.keys)

		var got string
		for i := 0; i < 20; i++ {
			got = page.MustEval(`() => document.querySelector('#t').value`).Str()
			if got == c.want {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if got != c.want {
			if b, err := page.Screenshot(false, nil); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "e2e-fail.png"), b, 0o644)
			}
			t.Fatalf("typed %q in %s: got %q, want %q (screenshot: e2e-fail.png)",
				c.keys, langNames[lang], got, c.want)
		}
		t.Logf("  %q -> %q OK", c.keys, got)
	}
}

func findBrowser() string {
	for _, p := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
