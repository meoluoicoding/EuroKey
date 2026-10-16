package engine

// Languages supported by the compose engine. Off disables transformation and
// passes every key straight through to the operating system.
const (
	LangOff = 0
	LangDE  = 1
	LangFR  = 2
	LangES  = 3
)

// LangIDs lists the languages in menu order.
var LangIDs = []int{LangOff, LangDE, LangFR, LangES}

// LangName is the display name for a language id (endonyms).
func LangName(id int) string {
	switch id {
	case LangDE:
		return "Deutsch"
	case LangFR:
		return "Français"
	case LangES:
		return "Español"
	}
	return "Off"
}

// langDef is the compose table for one language.
//
// dead maps a dead key to the base letters it combines with. The keys and
// base letters are the plain ASCII characters the hook reports, so the same
// dead keys work on any US/UK physical layout (US-International semantics:
// dead key + non-combining char emits the dead key literally).
type langDef struct {
	dead map[rune]map[rune]rune
}

var langs = map[int]langDef{
	LangDE: {dead: map[rune]map[rune]rune{
		'"': {'a': 'ä', 'o': 'ö', 'u': 'ü', 's': 'ß'},
	}},
	LangFR: {dead: map[rune]map[rune]rune{
		'\'': {'e': 'é'},
		'`':  {'a': 'à', 'e': 'è', 'u': 'ù'},
		'^':  {'a': 'â', 'e': 'ê', 'i': 'î', 'o': 'ô', 'u': 'û'},
		'"':  {'e': 'ë', 'i': 'ï', 'u': 'ü', 'y': 'ÿ'},
		',':  {'c': 'ç'},
	}},
	LangES: {dead: map[rune]map[rune]rune{
		'\'': {'a': 'á', 'e': 'é', 'i': 'í', 'o': 'ó', 'u': 'ú'},
		'~':  {'n': 'ñ'},
		'"':  {'u': 'ü'},
		'?':  {'?': '¿'},
		'!':  {'!': '¡'},
	}},
}
