#!/bin/sh
# Install EuroKey (IBus dead-key IME) for the current user.
#
#   ./install.sh            user install, no sudo (recommended)
#   ./install.sh --system   system-wide install under /usr (needs sudo)
#   ./uninstall.sh          remove
#
# Builds the binary with `go build` when the go toolchain is present;
# otherwise expects a prebuilt binary in ./dist.
set -eu

PREFIX="${PREFIX:-$HOME/.local/lib/eurokey}"
COMPONENT_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/ibus/component"
SUDO=""
if [ "${1:-}" = "--system" ]; then
	PREFIX=/usr/lib/eurokey
	COMPONENT_DIR=/usr/share/ibus/component
	SUDO=sudo
fi

HERE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
cd "$HERE"

if command -v go >/dev/null 2>&1; then
	echo "==> building eurokey-ibus"
	mkdir -p dist
	go build -trimpath -ldflags "-s -w" -o dist/eurokey-ibus ./cmd/eurokey-ibus
else
	echo "==> go not found; using prebuilt binary in ./dist"
fi
test -x dist/eurokey-ibus || { echo "missing dist/eurokey-ibus"; exit 1; }

$SUDO mkdir -p "$PREFIX" "$COMPONENT_DIR"
# install via rename: cp over a running binary fails with ETXTBSY, mv
# replaces it cleanly on the next upgrade
$SUDO cp dist/eurokey-ibus "$PREFIX/eurokey-ibus.new" && $SUDO mv -f "$PREFIX/eurokey-ibus.new" "$PREFIX/eurokey-ibus"
$SUDO cp ../logo.jpg "$PREFIX/logo.jpg"
sed "s|@PREFIX@|$PREFIX|g" data/eurokey.xml | $SUDO tee "$COMPONENT_DIR/eurokey.xml" >/dev/null

echo "==> installed to $PREFIX"
echo "Now restart ibus:  ibus restart"
echo "Then add 'EuroKey (Deutsch/Français/Español)' in Settings -> Keyboard -> Input Sources."
echo "Config dir: ${XDG_CONFIG_HOME:-$HOME/.config}/eurokey/ (eurokey.ini, macro.txt)"
