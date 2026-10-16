package session

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"eurokey/engine"
)

// sim folds each Action into a host: committed text accumulates, preedit is
// replaced. Forward is ignored (the app would insert the key itself).
type sim struct {
	text    string
	preedit string
}

func (s *sim) apply(a Action) {
	if a.Commit != nil {
		s.text += string(utf16.Decode(a.Commit))
	}
	s.preedit = ""
	if a.Preedit != nil {
		s.preedit = string(utf16.Decode(a.Preedit))
	}
}

func (s *sim) feed(sess *Session, keys string) {
	for _, r := range keys {
		a := sess.Feed(uint32(r))
		s.apply(a)
		if a.Forward {
			// the app inserts the forwarded key itself
			s.text += string(r)
		}
	}
}

func newSession(t *testing.T, lang int, enabled bool) (*Session, *engine.Engine) {
	t.Helper()
	eng := engine.New()
	eng.SetLanguage(lang)
	return New(eng, enabled), eng
}

func TestGermanComposeThenCommit(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.feed(sess, `gr"u`)
	if sm.preedit != "grü" {
		t.Fatalf("preedit = %q, want %q", sm.preedit, "grü")
	}
	sm.feed(sess, "n")
	if sm.preedit != "grün" {
		t.Fatalf("preedit = %q, want %q", sm.preedit, "grün")
	}
	sm.feed(sess, " ")
	if sm.text != "grün " {
		t.Fatalf("committed = %q, want %q", sm.text, "grün ")
	}
	if sm.preedit != "" {
		t.Fatalf("preedit after boundary = %q, want empty", sm.preedit)
	}
}

func TestGermanCapitalSharpS(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.feed(sess, `"S`)
	if sm.preedit != "ẞ" {
		t.Fatalf("preedit = %q, want ẞ", sm.preedit)
	}
}

func TestFrenchAndSpanish(t *testing.T) {
	fr, _ := newSession(t, engine.LangFR, true)
	sm := &sim{}
	sm.feed(fr, `'e`)
	if sm.preedit != "é" {
		t.Fatalf("french preedit = %q, want é", sm.preedit)
	}

	es, _ := newSession(t, engine.LangES, true)
	sm2 := &sim{}
	sm2.feed(es, `~n`)
	if sm2.preedit != "ñ" {
		t.Fatalf("spanish preedit = %q, want ñ", sm2.preedit)
	}
}

func TestDeadKeyFlushOnSpace(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.feed(sess, `" `)
	if sm.text != `"` {
		t.Fatalf("committed = %q, want %q", sm.text, `"`)
	}
	if sm.preedit != "" {
		t.Fatalf("preedit = %q, want empty", sm.preedit)
	}
}

func TestBackspaceCancelsDeadKey(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.apply(sess.Feed('"'))
	sm.apply(sess.Backspace())
	if sm.preedit != "" {
		t.Fatalf("preedit after cancelling dead key = %q, want empty", sm.preedit)
	}
	sm.feed(sess, "a")
	if sm.preedit != "a" {
		t.Fatalf("preedit = %q, want a", sm.preedit)
	}
}

func TestBackspaceDeletesPreedit(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.feed(sess, "ab")
	sm.apply(sess.Backspace())
	if sm.preedit != "a" {
		t.Fatalf("preedit after backspace = %q, want a", sm.preedit)
	}
	sm.feed(sess, " ")
	if sm.text != "a " {
		t.Fatalf("committed = %q, want %q", sm.text, "a ")
	}
}

func TestDisabledPassesThrough(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, false)
	a := sess.Feed('"')
	if a.Commit != nil || a.Preedit != nil || !a.Forward {
		t.Fatalf("disabled session must forward the key: %+v", a)
	}
}

func TestMacroExpansion(t *testing.T) {
	sess, eng := newSession(t, engine.LangDE, true)
	opts := eng.GetOptions()
	opts.MacroEnabled = 1
	eng.SetOptions(opts)
	path := filepath.Join(t.TempDir(), "macro.txt")
	if err := os.WriteFile(path, []byte("sg:Sehr geehrte\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !eng.LoadMacro(path) {
		t.Fatal("LoadMacro failed")
	}
	sm := &sim{}
	sm.feed(sess, "sg ")
	if sm.text != "Sehr geehrte " {
		t.Fatalf("committed = %q, want %q", sm.text, "Sehr geehrte ")
	}
}

func TestLanguageSwitchCommits(t *testing.T) {
	sess, _ := newSession(t, engine.LangDE, true)
	sm := &sim{}
	sm.feed(sess, "ab")
	// Switching language is done by the host via engine.SetLanguage + Reset;
	// the session just needs Reset to commit the pending word.
	sm.apply(sess.Reset())
	if sm.text != "ab" || sm.preedit != "" {
		t.Fatalf("reset: text=%q preedit=%q", sm.text, sm.preedit)
	}
}
