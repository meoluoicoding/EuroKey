package main

import (
	"testing"
	"time"
)

// benchKeyDownSetup isolates handleKeyDown from the real desktop: focus is
// faked and the engine is wrapped in a probe that never consumes, so
// injectOutput/SendInput never runs during the measurement.
func benchKeyDownSetup(tb testing.TB) {
	tb.Helper()
	oldEngine, oldCfg, oldLang, oldGlobal := gEngine, cfg, gLang, gEnabled
	oldSpec, oldExe, oldHwnd := lastFgSpec, lastFgExe, lastFgHwnd
	oldInputApp, oldKeyHwnd := gLastInputApp, gLastKeyHwnd
	tb.Cleanup(func() {
		gEngine, cfg, gLang, gEnabled = oldEngine, oldCfg, oldLang, oldGlobal
		lastFgSpec, lastFgExe, lastFgHwnd = oldSpec, oldExe, oldHwnd
		gLastInputApp, gLastKeyHwnd = oldInputApp, oldKeyHwnd
	})
	eng, _ := newEngine()
	eng.SetLanguage(LangDE)
	cfg = config{}
	gLang = LangDE
	gEnabled = true
	focusAppForTest("bench.exe", appSpec{lang: -1})
	gEngine = &typingModeProbe{Engine: eng, on: true}
}

var benchKeys = []uint32{'G', 'R', 'O', 'S', 'S', 'E', ' ', 'S', 'T', 'R', 'A', 'S', 'S', 'E'}

func BenchmarkHandleKeyDown(b *testing.B) {
	benchKeyDownSetup(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, vk := range benchKeys {
			handleKeyDown(&kbdLLHookStruct{vkCode: vk})
		}
	}
}

// TestHandleKeyDownLatency guards the low-level hook budget: Windows removes a
// hook whose callback exceeds LowLevelHooksTimeout (default 300ms), and a
// laggy callback makes typing feel stuck. We require well under 1 ms per key.
func TestHandleKeyDownLatency(t *testing.T) {
	benchKeyDownSetup(t)
	const n = 20000
	start := time.Now()
	for i := 0; i < n; i++ {
		handleKeyDown(&kbdLLHookStruct{vkCode: benchKeys[i%len(benchKeys)]})
	}
	elapsed := time.Since(start)
	per := elapsed / n
	t.Logf("handleKeyDown: %v in total, %v/key over %d keys", elapsed, per, n)
	if per > time.Millisecond {
		t.Fatalf("handleKeyDown too slow for a low-level hook: %v/key", per)
	}
}
