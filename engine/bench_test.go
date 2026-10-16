package engine

import (
	"strings"
	"testing"
	"time"
)

// benchStream types text through the engine once per iteration. The engine is
// reset between iterations so dead-key state never accumulates across b.N.
func benchStream(b *testing.B, lang int, text string) {
	e := New()
	e.SetLanguage(lang)
	keys := []rune(text)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, k := range keys {
			e.Filter(uint32(k))
		}
		e.Reset()
	}
}

func BenchmarkFilterGerman(b *testing.B) {
	benchStream(b, LangDE, "\"o\"se Stra\"se m\"ochte \"uber \"Anderungen")
}

func BenchmarkFilterFrench(b *testing.B) {
	benchStream(b, LangFR, "'et`e ,c ^a va ^etre tr`es")
}

func BenchmarkFilterSpanish(b *testing.B) {
	benchStream(b, LangES, "~n 'a ?? !! a'u ni~no")
}

// Off is the pass-through path: the cheapest case, a useful baseline.
func BenchmarkFilterOff(b *testing.B) {
	benchStream(b, LangOff, "plain english text here 123")
}

func BenchmarkBackspace(b *testing.B) {
	e := New()
	e.SetLanguage(LangDE)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Filter('"')
		e.Backspace()
	}
}

// TestTypingThroughput measures sustained keys/second through the engine. A
// human typing fast bursts at ~10-20 keys/s, so the floor leaves a huge margin
// to catch a catastrophic regression rather than to be exact.
func TestTypingThroughput(t *testing.T) {
	e := New()
	e.SetLanguage(LangDE)
	const phrase = "gr\"o\"se Stra\"se m\"ochte \"uber \"Anderungen "
	keys := []rune(strings.Repeat(phrase, 4000))
	if len(keys) < 100000 {
		t.Fatalf("sample too small: %d keys", len(keys))
	}

	start := time.Now()
	for _, k := range keys {
		e.Filter(uint32(k))
	}
	elapsed := time.Since(start)
	perKey := elapsed / time.Duration(len(keys))
	perSec := float64(len(keys)) / elapsed.Seconds()
	t.Logf("engine: %d keys in %v (%.3f us/key, %.0f keys/s)",
		len(keys), elapsed, float64(perKey.Nanoseconds())/1000, perSec)
	if perSec < 100000 {
		t.Fatalf("engine throughput too low: %.0f keys/s (< 100000)", perSec)
	}
}
