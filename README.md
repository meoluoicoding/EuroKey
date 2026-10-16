# EuroKey

Bàn phím ký tự châu Âu (Đức · Pháp · Tây Ban Nha) cho **Windows**, nhập ký tự có
dấu bằng **dead key / compose** — gõ trên bàn phím US/UK vẫn ra ä, é, ñ…

Fork từ [Tui Gõ](https://github.com/captajn/TuiGoTiengViet), dùng lại bộ khung
Win32 (keyboard hook, khay hệ thống, HUD, cửa sổ cài đặt) và thay engine biến đổi
tiếng Việt bằng engine compose đa ngôn ngữ.

## Cách gõ

Chọn ngôn ngữ trong Cài đặt hoặc menu khay. Sau đó gõ một ký tự "chết" rồi tới
ký tự nền (giống US-International). Gõ ký tự chết rồi Space để ra chính ký tự đó.

| Ngôn ngữ | Gõ | Ra |
|---|---|---|
| Deutsch | `"` `a` / `o` / `u` / `s` | ä / ö / ü / ß |
| Français | `'` `e` | é |
| | `` ` `` `a` / `e` / `u` | à / è / ù |
| | `^` `a` `e` `i` `o` `u` | â ê î ô û |
| | `,` `c` | ç |
| | `"` `e` `i` `u` `y` | ë ï ü ÿ |
| Español | `'` `a` `e` `i` `o` `u` | á é í ó ú |
| | `~` `n` | ñ |
| | `"` `u` | ü |
| | `?` `?` / `!` `!` | ¿ / ¡ |

Chữ hoa được giữ nguyên: `"A` → Ä. Backspace huỷ dead key đang chờ.

## Tính năng

- Ba ngôn ngữ Đức / Pháp / Tây Ban Nha + `Off` (mọi phím đi thẳng).
- Dead key / compose, không đổi layout vật lý (chạy trên mọi bố cục).
- Gõ tắt (macro) dạng `từ_tắt:nội_dung` trong `macro.txt`.
- Khay hệ thống, HUD trạng thái, phím nóng bật/tắt, phím nhanh F1/F2/F5/F9/F12.
- Ứng dụng loại trừ + tự nhớ trạng thái bật/tắt theo ứng dụng (`exclude.txt`).
- Giao diện sáng/tối + nhiều phong cách chủ đề.

## Build

Yêu cầu Go 1.23+ (không cần toolchain C++).

```powershell
go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
```

hoặc:

```bat
build.bat          rem -> eurokey.exe (GUI)
build.bat test     rem -> chạy test engine + shell
build.bat console  rem -> eurokey.exe kèm console để debug
```

## Dữ liệu cục bộ

Portable — mọi thứ nằm cạnh `eurokey.exe`:

- `eurokey.ini` — cài đặt (ngôn ngữ, macro, phím nóng, chủ đề, app loại trừ).
- `macro.txt` — bảng gõ tắt, mỗi dòng `từ_tắt:nội_dung`.
- `exclude.txt` — app loại trừ, mỗi dòng một tên `.exe` kèm cờ tuỳ chọn
  (`lock`, `clip`, `safedel`, `de`/`fr`/`es`).

Không có lịch sử gõ, không telemetry.

## Quyền riêng tư

- Xử lý hoàn toàn trên máy; app gõ không có HTTP client.
- Chỉ hook bàn phím/chuột phục vụ gõ và phím tắt; chế độ Clipboard đọc/thay
  clipboard tạm thời để gửi chữ, có cơ chế khôi phục.
- Repo không kèm binary updater; nút "Kiểm tra cập nhật" chỉ hoạt động nếu bạn
  đặt `eurokey-updater.exe` cạnh app (chưa phát hành).

## Nền tảng

### Windows
App chính (`eurokey.exe`): hook bàn phím mức thấp, khay hệ thống, HUD, cửa sổ cài đặt tự vẽ.

### Linux (IBus)
`linux/` — engine IBus thuần Go, dùng chung `engine/`:

| Thành phần | Vai trò |
|---|---|
| `linux/cmd/eurokey-ibus` | IBus engine (preedit + commit), menu bật/tắt + chọn ngôn ngữ |
| `linux/internal/session` | giữ từ đang soạn trong preedit (IBus không xoá được text đã commit) |
| `linux/config` | `~/.config/eurokey/eurokey.ini` + `macro.txt` |
| `linux/data/eurokey.xml` | component descriptor |
| `linux/install.sh` / `uninstall.sh` | cài/gỡ cho user (hoặc `--system`) |

```sh
cd linux
./install.sh        # hoặc ./install.sh --system
ibus restart
# Settings -> Keyboard -> Input Sources -> "EuroKey (Deutsch/Français/Español)"
```

Cross-build từ Windows/macOS: `GOOS=linux go build ./cmd/eurokey-ibus`.

Chưa verify runtime trên Ubuntu (cần IBus thật) — xem `checks.md`.

Logo hiện dùng tạm mascot của Tui Gõ.
