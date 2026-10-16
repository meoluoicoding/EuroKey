package main

import (
	"eurokey/engine"
)

// Engine abstracts the compose engine implemented by package engine.
type Engine interface {
	Filter(ch uint32) Result // feed one ASCII char
	Backspace() Result
	Reset()
	SetEnabled(on bool) // off = pass-through, macros still expand
	SetLanguage(lang int)
	GetOptions() Options
	SetOptions(o Options)
	LoadMacro(path string) bool
}

// Result of processing one key.
type Result = engine.Result

type Options = engine.Options

// Languages (compose tables).
const (
	LangOff = engine.LangOff
	LangDE  = engine.LangDE
	LangFR  = engine.LangFR
	LangES  = engine.LangES
)

// LangIDs lists the languages in menu order.
var LangIDs = engine.LangIDs

// LangName is the display name for a language id.
func LangName(id int) string { return engine.LangName(id) }

// ---- compose engine -------------------------------------------------------

type goEngine struct {
	e *engine.Engine
}

func newEngine() (*goEngine, error) {
	return &goEngine{e: engine.New()}, nil
}

func (g *goEngine) Filter(ch uint32) Result { return g.e.Filter(ch) }
func (g *goEngine) Backspace() Result       { return g.e.Backspace() }
func (g *goEngine) Reset()                  { g.e.Reset() }

func (g *goEngine) SetEnabled(on bool)   { g.e.SetEnabled(on) }
func (g *goEngine) SetLanguage(lang int) { g.e.SetLanguage(lang) }

func (g *goEngine) GetOptions() Options { return g.e.GetOptions() }
func (g *goEngine) SetOptions(o Options) { g.e.SetOptions(o) }

func (g *goEngine) LoadMacro(p string) bool { return g.e.LoadMacro(p) }
