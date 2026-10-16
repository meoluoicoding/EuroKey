# checks.md — EuroKey test evidence

Windows fork (custom shell reused from Tui Gõ + new compose engine). Go 1.26.4.
All commands run from the repo root unless noted.

## Cách chạy

| Mức | Lệnh | Kiểm gì |
|---|---|---|
| Unit | `go test ./engine/ ` | Engine compose: dead key, ghép dấu, flush, macro (không cần GUI) |
| Unit | `go test . -run "TestUi" -skip TestUi` (hoặc bỏ `-skip`) | Logic shell: app mode, tray, settings merge |
| UI | `go test . -run TestUi -v` | Cửa sổ Cài đặt thật: click radio/combo/switch qua `SendMessage(WM_COMMAND)` |
| Smoke | `go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .` | Biên dịch + link toàn bộ, ra exe chạy được |
| Tổng | `powershell -File scripts/check-eurokey.ps1` | vet + toàn bộ test + build |

- UI test mở cửa sổ Cài đặt thật; tự **skip** nếu không có phiên desktop tương tác.
- Dùng `-NoUi` để bỏ qua nhóm UI khi chạy headless.

## Kết quả

### Unit tests

```
go test ./...
ok   eurokey          <shell>
ok   eurokey/engine   13 test (compose DE/FR/ES, flush, cancel dead key, macro, SetEnabled per-key)
```

- `engine/engine_test.go`: `TestGermanUmlaut`, `TestGermanUmlautUppercase`,
  `TestGermanRingelS`, `TestGermanCapitalSharpS` (`"S` → ẞ U+1E9E),
  `TestFrenchAcuteAndCedilla`, `TestSpanishNtildeAndInverted`,
  `TestDeadKeyFlushWhenNoCombination`, `TestDeadKeyPlusSpaceEmitsLiteral`,
  `TestBackspaceCancelsDeadKey`, `TestPassThroughWhenOff`,
  `TestMacroExpansion`, `TestMacroReloadClearsRemovedFile`,
  `TestSetEnabledPerKeyKeepsDeadKey`.
- Hồi quy đã sửa: `SetEnabled` gọi mỗi phím từng xoá dead-key đang chờ
  (`"a` không ra `ä`). Test `TestSetEnabledPerKeyKeepsDeadKey` mô phỏng đúng cách
  shell gọi và pass.

### UI tests (in-process, WM_COMMAND thật)

```
go test . -run TestUi -v
--- PASS: TestUiClickLanguageRadios (Deutsch/Francais/Espanol/Off)
--- PASS: TestUiComboLanguage
--- PASS: TestUiMacroToggle
PASS  ok  eurokey
```

Bẫy đã gặp: `settingsProc` có `defer recover()` nên khi `gEngine` nil trong tiến
trình test, click "im lặng" không đổi state. Helper `uiOpenSettings` nay tự tạo
engine — không có nó, test dễ xanh giả.

### Build / smoke

```
go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
# -> eurokey.exe (~3.7 MB)
```

### Tốc độ gõ (perf)

```powershell
go test ./engine/ -bench . -benchtime=200ms -run "^$"   # engine ns/op
go test ./engine/ -run TestTypingThroughput -v          # keys/s
go test . -run TestHandleKeyDownLatency -v              # hook us/key
```

Kết quả (Intel i5-10300H):

| Đo | Số | Nghĩa |
|---|---|---|
| `Filter` Deutsch | 686 ns/key | 1 phím qua engine (có dead key) |
| `Filter` Français / Español | 599 / 567 ns/key | |
| `Filter` Off | 119 ns/key | baseline pass-through |
| `Backspace` | 14 ns | |
| Throughput | 0.026 µs/key (~38.000.000 keys/s) | 168.000 phím trong 4.4 ms |
| `handleKeyDown` (hook) | 4.2 µs/key | gồm `ToUnicodeEx` + foreground app |

Diễn giải: người gõ nhanh ~8-20 phím/giây (50 ms/phím). App thêm ~0.004 ms/phím
→ ~0.008% ngân sách. Windows gỡ hook nếu callback > `LowLevelHooksTimeout`
(mặc định 300 ms); ta thấp hơn ~70.000 lần.

Giới hạn: số trên là **phía app** (hook + engine). Độ trễ thật còn gồm `SendInput`
và app đích → muốn đo WPM thật phải gõ tay hoặc viết driver dùng
`EUROKEY_TEST_MAGIC`.

### E2E gõ thật (rod + SendInput)

```powershell
# Đóng EuroKey trước (single-instance + cần đặt EUROKEY_TEST_MAGIC lúc khởi động)
powershell -File scripts/e2e-eurokey.ps1            # ngôn ngữ đọc từ eurokey.ini
powershell -File scripts/e2e-eurokey.ps1 -Lang de   # ép Deutsch
```

`e2e/e2e_test.go` (module riêng `eurokey/e2e`, dùng `github.com/go-rod/rod`):

- **rod** = mở Chromium + trang `data:` có `<textarea>`, đọc lại `value`, chụp
  screenshot khi fail.
- **Gõ** thì KHÔNG dùng rod: sự kiện phím của CDP không đi qua low-level hook
  của Windows, nên EuroKey sẽ không xử lý. Thay vào đó harness gửi phím bằng
  `SendInput` ở mức OS, gắn `dwExtraInfo = EUROKEY_TEST_MAGIC` (hook chấp nhận
  như phím thật — xem `features.go`).
- Tự **skip** khi: chưa đặt `EUROKEY_E2E=1`, app đang chạy, thiếu Chrome/Edge,
  hoặc không đưa được cửa sổ browser lên foreground.

Trạng thái: harness **biên dịch + chạy + skip đúng** khi app đang mở. **Chưa có
lượt PASS thật** vì app đang chạy lúc kiểm thử — cần đóng app rồi chạy lại.

### E2E thủ công (dự phòng)

Chạy app thật, gõ vào Notepad/Word/trình duyệt. Đánh dấu ✅/❌:

- [ ] Tray có icon EuroKey; chuột phải → menu có `Ngôn ngữ → Off/Deutsch/Français/Español`.
- [ ] Chọn **Deutsch**, gõ `"a` → `ä`; `"o` → `ö`; `"u` → `ü`; `"s` → `ß`; `"S` → `ẞ`.
- [ ] Gõ `"` rồi Space → ra `"`.
- [ ] Gõ `"` rồi `x` → ra `"x`.
- [ ] Gõ `"` rồi Backspace → huỷ dead key, không xoá ký tự.
- [ ] Chọn **Français**: `'e`→é, `,c`→ç, `^e`→ê.
- [ ] Chọn **Español**: `~n`→ñ, `'a`→á, `??`→¿, `!!`→¡.
- [ ] Chọn **Off**: mọi phím đi thẳng (`"a` → `"a`).
- [ ] Alt+Z (hoặc Ctrl+Shift) bật/tắt; HUD/tray đổi màu.
- [ ] Gõ tắt: bật macro, thêm `sg:Sehr geehrte` vào `macro.txt`, gõ `sg ` → bung.
- [ ] Cài đặt: đổi theme sáng/tối; mở lại app nhớ đúng ngôn ngữ/theme.

### Cơ chế e2e tự động (tuỳ chọn, chưa làm)

`EUROKEY_TEST_MAGIC` (trong `features.go`): khi đặt biến môi trường này bằng một
số khác 0, phím do `SendInput` gắn đúng `dwExtraInfo = magic` sẽ được app xử lý
**như phím thật** (đi qua engine + probe + injection). Có thể viết driver Go/PS
gửi phím vào cửa sổ đích rồi đọc lại nội dung để e2e thật. Hiện chưa có driver.

## Linux (IBus port)

```sh
cd linux
go test ./config/ ./internal/session/          # pure Go — chạy cả trên Windows/CI
sh scripts/check-linux.sh                      # trên Ubuntu: vet + test + build amd64/arm64
```

Kết quả (2026-10-16, cross-compile từ Windows):

| Bước | Kết quả |
|---|---|
| `go test ./config/ ./internal/session/` | PASS (native, không cần Linux) |
| `GOOS=linux go vet ./...` | PASS |
| build `dist/eurokey-ibus` (amd64) + `dist/eurokey-ibus-arm64` | PASS |
| GitHub Actions job `linux` (ubuntu-latest, `scripts/check-linux.sh`) | PASS |

Test phủ: session (compose DE/FR/ES, flush dead key, backspace, macro, pass-through
khi tắt), config (defaults, round-trip, giữ key lạ, toggle keys).

**Chưa verify runtime trên Ubuntu** — cross build chỉ chứng minh biên dịch; cần
IBus thật để xác nhận commit/preedit. Không claim IBus đã chạy.

## Chưa xác minh (unresolved)

- **Chưa xem screenshot** cửa sổ Cài đặt/tray/HUD → chưa chứng minh layout đúng.
  Build + test không thay được kiểm tra thị giác.
- **Chưa chạy e2e gõ thật** bằng tay ở mọi app; chỉ mới unit + UI in-process.
- Icon/mascot vẫn là của Tui Gõ (chưa rebrand); `rsrc_amd64.syso` chưa regenerate.

Lịch sử chạy:

| Ngày | Lệnh | Kết quả |
|---|---|---|
| 2026-10-16 | `go test ./...` | PASS (engine + shell) |
| 2026-10-16 | `go test . -run TestUi -v` | PASS (3 nhóm UI) |
| 2026-10-16 | `go build -o eurokey.exe .` | PASS |
| 2026-10-16 | GitHub Actions `build` (push main) | PASS — test + build amd64/arm64 |
| 2026-10-16 | GitHub Actions `release` (tag `v0.1.0`) | PASS — 2 zip assets |
| 2026-10-16 | Linux port: `go test ./config ./internal/session` + cross build | PASS (chưa chạy IBus) |
