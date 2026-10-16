package engine

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// docSim replays engine results the way the Windows shell would: apply the
// backspaces, append the injected text, and append the key itself when the
// engine did not consume it (the OS typed it).
type docSim struct {
	text []uint16
}

func (d *docSim) apply(res Result) {
	if res.Backs > 0 {
		if res.Backs >= len(d.text) {
			d.text = d.text[:0]
		} else {
			d.text = d.text[:len(d.text)-res.Backs]
		}
	}
	d.text = append(d.text, res.Out...)
}

func (d *docSim) feed(e *Engine, key rune) {
	res := e.Filter(uint32(key))
	d.apply(res)
	if !res.Consumed {
		d.text = append(d.text, uint16(key))
	}
}

func (d *docSim) typeKeys(e *Engine, keys string) {
	for _, k := range keys {
		d.feed(e, k)
	}
}

func (d *docSim) String() string { return string(utf16.Decode(d.text)) }

func newSim(lang int) (*Engine, *docSim) {
	e := New()
	e.SetLanguage(lang)
	return e, &docSim{}
}

// The Windows shell calls SetEnabled on every keydown; a dead key set by one
// keydown must survive the next keydown's SetEnabled call (regression: "a did
// not compose to ä because SetEnabled reset the pending state).
func TestSetEnabledPerKeyKeepsDeadKey(t *testing.T) {
	e := New()
	e.SetLanguage(LangDE)
	d := &docSim{}
	for _, k := range `"a` {
		e.SetEnabled(true) // shell does this before every Filter
		d.feed(e, k)
	}
	if got := d.String(); got != "ä" {
		t.Fatalf(`repeated SetEnabled wiped the dead key: %q, want "ä"`, got)
	}
}

func TestGermanUmlaut(t *testing.T) {
	e, d := newSim(LangDE)
	d.typeKeys(e, `"a`)
	if got := d.String(); got != "ä" {
		t.Fatalf(`"a = %q, want "ä"`, got)
	}
}

func TestGermanUmlautUppercase(t *testing.T) {
	e, d := newSim(LangDE)
	d.typeKeys(e, `"O`)
	if got := d.String(); got != "Ö" {
		t.Fatalf(`"O = %q, want "Ö"`, got)
	}
}

func TestGermanRingelS(t *testing.T) {
	e, d := newSim(LangDE)
	d.typeKeys(e, `"s`)
	if got := d.String(); got != "ß" {
		t.Fatalf(`"s = %q, want "ß"`, got)
	}
}

func TestGermanCapitalSharpS(t *testing.T) {
	e, d := newSim(LangDE)
	d.typeKeys(e, `"S`)
	if got := d.String(); got != "ẞ" {
		t.Fatalf(`"S = %q, want "ẞ" (U+1E9E)`, got)
	}
}

func TestFrenchAcuteAndCedilla(t *testing.T) {
	e, d := newSim(LangFR)
	d.typeKeys(e, `'e`)
	if got := d.String(); got != "é" {
		t.Fatalf(`'e = %q, want "é"`, got)
	}
	d2 := &docSim{}
	d2.typeKeys(e, `,c`)
	if got := d2.String(); got != "ç" {
		t.Fatalf(`,c = %q, want "ç"`, got)
	}
}

func TestSpanishNtildeAndInverted(t *testing.T) {
	e, d := newSim(LangES)
	d.typeKeys(e, `~n`)
	if got := d.String(); got != "ñ" {
		t.Fatalf(`~n = %q, want "ñ"`, got)
	}
	d2 := &docSim{}
	d2.typeKeys(e, `??`)
	if got := d2.String(); got != "¿" {
		t.Fatalf(`?? = %q, want "¿"`, got)
	}
	d3 := &docSim{}
	d3.typeKeys(e, `!!`)
	if got := d3.String(); got != "¡" {
		t.Fatalf(`!! = %q, want "¡"`, got)
	}
}

func TestDeadKeyFlushWhenNoCombination(t *testing.T) {
	e, d := newSim(LangES)
	d.typeKeys(e, `~x`)
	if got := d.String(); got != "~x" {
		t.Fatalf(`~x = %q, want "~x"`, got)
	}
}

func TestDeadKeyPlusSpaceEmitsLiteral(t *testing.T) {
	e, d := newSim(LangFR)
	d.typeKeys(e, `' `)
	if got := d.String(); got != "'" {
		t.Fatalf(`'<space> = %q, want "'"`, got)
	}
}

func TestBackspaceCancelsDeadKey(t *testing.T) {
	e, d := newSim(LangDE)
	if res := e.Filter('"'); !res.Consumed || len(res.Out) != 0 {
		t.Fatalf("dead key should be swallowed: %+v", res)
	}
	if res := e.Backspace(); !res.Consumed {
		t.Fatalf("backspace should cancel the dead key, got %+v", res)
	}
	if got := d.String(); got != "" {
		t.Fatalf("text = %q, want empty", got)
	}
	// The next character is now a plain pass-through.
	if res := e.Filter('a'); res.Consumed {
		t.Fatalf("'a' after cancelled dead key should pass through: %+v", res)
	}
}

func TestPassThroughWhenOff(t *testing.T) {
	e := New()
	e.SetLanguage(LangOff)
	if res := e.Filter('"'); res.Consumed {
		t.Fatalf("off language must pass through: %+v", res)
	}
	e2 := New()
	e2.SetLanguage(LangDE)
	e2.SetEnabled(false)
	if res := e2.Filter('"'); res.Consumed {
		t.Fatalf("disabled engine must pass through: %+v", res)
	}
}

func TestMacroExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "macro.txt")
	if err := os.WriteFile(path, []byte("sg:Sehr geehrte\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e, d := newSim(LangDE)
	e.SetOptions(Options{MacroEnabled: 1})
	if !e.LoadMacro(path) {
		t.Fatal("LoadMacro failed")
	}
	d.typeKeys(e, "sg ")
	if got := d.String(); got != "Sehr geehrte " {
		t.Fatalf("macro = %q, want %q", got, "Sehr geehrte ")
	}
}

func TestMacroReloadClearsRemovedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "macro.txt")
	if err := os.WriteFile(path, []byte("sg:Sehr geehrte\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e, _ := newSim(LangDE)
	e.SetOptions(Options{MacroEnabled: 1})
	e.LoadMacro(path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	e.LoadMacro(path)
	if _, ok := e.macro.lookup("sg"); ok {
		t.Fatal("macro from a removed file must not survive a reload")
	}
}
