package engine

import (
	"bufio"
	"os"
	"strings"
)

// macroStore is a simple "shortcut:expansion" text substitution table loaded
// from a UTF-8 macro.txt file (one entry per line, ; and # comments).
type macroStore struct {
	items map[string]string
}

func (m *macroStore) load(path string) bool {
	items := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		// A removed or unreadable file empties the list; existing items are
		// never kept from a previous load.
		m.items = items
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		items[key] = strings.TrimSpace(value)
	}
	m.items = items
	return true
}

func (m *macroStore) lookup(key string) (string, bool) {
	v, ok := m.items[key]
	return v, ok
}
