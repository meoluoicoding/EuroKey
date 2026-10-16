// Package engine implements the EuroKey compose engine: dead-key sequences
// that turn plain ASCII keystrokes into German, French and Spanish accented
// characters. It mirrors the buffer -> transform -> commit contract the
// Windows shell expects: Filter returns the text to inject plus how many
// characters must be deleted first, and whether the shell should swallow the
// physical key.
package engine

import (
	"unicode"
	"unicode/utf16"
)

// Options mirror the shell's persisted engine flags.
type Options struct {
	MacroEnabled int32
}

// Result of processing one key, in the shape the Windows hook injects.
type Result struct {
	Out      []uint16 // text to inject (UTF-16 units)
	Backs    int      // characters to delete before injecting Out
	Consumed bool     // true = the shell must not pass the key through
}

// Engine holds the compose state machine.
type Engine struct {
	opts    Options
	lang    int
	enabled bool

	pending rune   // held dead key (0 = none)
	word    []rune // current ASCII word, for macros
	macro   macroStore
}

// New returns an enabled engine with language off and no macros.
func New() *Engine {
	return &Engine{enabled: true}
}

// SetEnabled turns transformation on or off. Off passes every key through.
// It is a no-op when the value is unchanged: the shell calls it on every
// keydown, and resetting there would wipe a pending dead key before the next
// character can combine with it.
func (e *Engine) SetEnabled(on bool) {
	if e.enabled == on {
		return
	}
	e.enabled = on
	e.resetState()
}

// SetLanguage selects the compose table (LangOff disables composition). It is a
// no-op when unchanged, for the same reason as SetEnabled.
func (e *Engine) SetLanguage(lang int) {
	if e.lang == lang {
		return
	}
	e.lang = lang
	e.resetState()
}

// Reset drops the pending dead key and the macro word buffer.
func (e *Engine) Reset() { e.resetState() }

func (e *Engine) resetState() {
	e.pending = 0
	e.word = e.word[:0]
}

// GetOptions returns the current engine options.
func (e *Engine) GetOptions() Options { return e.opts }

// SetOptions replaces the engine options.
func (e *Engine) SetOptions(o Options) { e.opts = o }

// LoadMacro (re)loads the macro table from path. An unreadable file empties it.
func (e *Engine) LoadMacro(path string) bool { return e.macro.load(path) }

// Filter feeds one ASCII character and returns what to inject. A non-consumed
// result means the shell should let the operating system type the key itself.
func (e *Engine) Filter(ch uint32) Result {
	if !e.enabled || e.lang == LangOff {
		e.resetState()
		return Result{}
	}
	def, ok := langs[e.lang]
	if !ok {
		e.resetState()
		return Result{}
	}
	r := rune(ch)

	// Complete a held dead key.
	if e.pending != 0 {
		dead := e.pending
		e.pending = 0
		if comb, ok := def.dead[dead]; ok {
			if mapped, ok := comb[unicode.ToLower(r)]; ok {
				e.resetState()
				return Result{Out: []uint16{uint16(applyCase(r, mapped))}, Consumed: true}
			}
		}
		// US-International: dead key + Space emits the dead key alone.
		if r == ' ' {
			e.resetState()
			return Result{Out: []uint16{uint16(dead)}, Consumed: true}
		}
		// No combination: emit the dead key literally, then this character.
		e.trackChar(r)
		return Result{Out: []uint16{uint16(dead), uint16(r)}, Consumed: true}
	}

	// Start a dead key: hold it, inject nothing.
	if _, isDead := def.dead[r]; isDead {
		e.pending = r
		return Result{Consumed: true}
	}

	// Macro expansion on the terminating space.
	if r == ' ' && e.opts.MacroEnabled != 0 && len(e.word) > 0 {
		if exp, ok := e.macro.lookup(string(e.word)); ok {
			backs := len(e.word)
			e.resetState()
			out := utf16Of(exp)
			out = append(out, ' ')
			return Result{Out: out, Backs: backs, Consumed: true}
		}
	}

	e.trackChar(r)
	return Result{}
}

// Backspace cancels a pending dead key (swallowing the key so nothing is
// deleted, since the dead key was never emitted). Otherwise it is an ordinary
// OS backspace and only the macro word buffer is trimmed.
func (e *Engine) Backspace() Result {
	if e.pending != 0 {
		e.pending = 0
		return Result{Consumed: true}
	}
	if n := len(e.word); n > 0 {
		e.word = e.word[:n-1]
	}
	return Result{}
}

func (e *Engine) trackChar(r rune) {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		e.word = append(e.word, r)
		return
	}
	e.word = e.word[:0]
}

// applyCase preserves the case of the typed base letter (A -> Ä, a -> ä).
// ß is special-cased: Go's simple case mapping leaves it unchanged, but the
// capital sharp s (ẞ, U+1E9E) is the correct uppercase form.
func applyCase(base, mapped rune) rune {
	if unicode.IsUpper(base) {
		if mapped == 'ß' {
			return 'ẞ'
		}
		return unicode.ToUpper(mapped)
	}
	return mapped
}

func utf16Of(s string) []uint16 { return utf16.Encode([]rune(s)) }
