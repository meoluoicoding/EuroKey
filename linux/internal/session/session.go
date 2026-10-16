// Package session adapts the engine's edit-stream contract
// (Result{Backs, Out}) to IBus, which cannot delete already-committed text.
//
// Unlike the word-based Vietnamese engine, EuroKey's engine is a dead-key
// composer: pressing a dead key holds it (Consumed, no output), and the next
// character either combines into an accented one or flushes the dead key
// literally. The current word is kept in preedit and committed at a word
// boundary (space, punctuation, focus loss) or when a macro expands.
package session

import (
	"unicode/utf16"

	"eurokey/engine"
)

// Action is what the host must do after one key event, in order:
//  1. insert Commit at the caret (replacing any visible preedit),
//  2. show Preedit as marked text (empty hides the preedit),
//  3. let the original key through to the app if Forward is set.
type Action struct {
	Commit  []uint16 // UTF-16 units to commit; may be empty
	Preedit []uint16 // marked text to display after committing
	Forward bool     // forward the triggering key to the application
}

// Session wraps one engine.Engine for one client context.
type Session struct {
	eng     *engine.Engine
	model   []uint16 // rendered form of the current composing word
	enabled bool
	AutoCap bool // capitalise the first letter after ". ", "! ", "? "
	capNext bool
}

// New creates a session. enabled=false means pass-through.
func New(eng *engine.Engine, enabled bool) *Session {
	eng.SetEnabled(enabled)
	return &Session{eng: eng, enabled: enabled}
}

// SetEnabled switches composition on/off, committing any pending word.
func (s *Session) SetEnabled(v bool) Action {
	if v == s.enabled {
		return Action{}
	}
	s.enabled = v
	s.eng.SetEnabled(v)
	s.eng.Reset()
	return s.commit()
}

// isWordChar reports whether a character belongs to the composing word (so the
// session swallows it into preedit instead of committing on it).
func isWordChar(ch uint32) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' ||
		ch == '\'' || ch == '-' || ch == '_'
}

// Feed processes one printable character (Unicode codepoint).
func (s *Session) Feed(ch uint32) Action {
	if !s.enabled {
		return s.forward()
	}
	forceCap := s.AutoCap && s.capNext && ch >= 'a' && ch <= 'z'
	if forceCap {
		ch -= 'a' - 'A'
	}
	res := s.eng.Filter(ch)

	if s.AutoCap {
		switch ch {
		case '.', '!', '?':
			s.capNext = true
		case ' ':
			// pending capitalisation survives whitespace
		default:
			s.capNext = false
		}
	}

	if res.Consumed {
		if len(res.Out) == 0 && res.Backs == 0 {
			// A dead key is held; nothing visible changes.
			return s.preedit()
		}
		s.apply(res.Backs, res.Out)
		if !isWordChar(ch) {
			// Boundary consumed by the engine (dead-key flush on space, macro
			// expansion): commit the whole word now.
			return s.commit()
		}
		return s.preedit()
	}

	if isWordChar(ch) {
		// Engine recorded it but passed it through: swallow into preedit so a
		// later dead key / macro can still rewrite the word.
		s.model = append(s.model, utf16.Encode([]rune{rune(ch)})...)
		return s.preedit()
	}
	// Word boundary: commit the word and let the app insert the key itself.
	a := s.commit()
	a.Forward = true
	return a
}

// Backspace forwards one delete press through the engine.
func (s *Session) Backspace() Action {
	if !s.enabled {
		return s.forward()
	}
	res := s.eng.Backspace()
	if res.Consumed {
		// Cancelled a pending dead key — nothing visible changed.
		return s.preedit()
	}
	if len(s.model) == 0 {
		return s.forward()
	}
	s.model = s.model[:len(s.model)-lastRuneUnits(s.model)]
	return s.preedit()
}

// Reset commits any pending word and drops the engine buffer — call it on
// focus loss, caret movement and modifier shortcuts.
func (s *Session) Reset() Action {
	s.capNext = false
	s.eng.Reset()
	return s.commit()
}

// EnterPressed marks a paragraph break for AutoCap and resets.
func (s *Session) EnterPressed() Action {
	if s.AutoCap {
		s.capNext = true
	}
	return s.Reset()
}

func (s *Session) apply(backs int, out []uint16) {
	if backs > len(s.model) {
		backs = len(s.model)
	}
	s.model = s.model[:len(s.model)-backs]
	s.model = append(s.model, out...)
}

func (s *Session) preedit() Action {
	return Action{Preedit: append([]uint16(nil), s.model...)}
}

func (s *Session) commit() Action {
	a := Action{Commit: append([]uint16(nil), s.model...)}
	s.model = s.model[:0]
	return a
}

func (s *Session) forward() Action {
	a := s.commit()
	a.Forward = true
	return a
}

// lastRuneUnits returns how many UTF-16 units the last rune occupies.
func lastRuneUnits(u []uint16) int {
	if len(u) == 0 {
		return 0
	}
	if len(u) >= 2 && u[len(u)-1] >= 0xDC00 && u[len(u)-1] <= 0xDFFF &&
		u[len(u)-2] >= 0xD800 && u[len(u)-2] <= 0xDBFF {
		return 2
	}
	return 1
}
