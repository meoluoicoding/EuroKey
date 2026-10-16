#!/bin/sh
# Remove a user install of EuroKey.
set -eu

PREFIX="${PREFIX:-$HOME/.local/lib/eurokey}"
COMPONENT_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/ibus/component"

rm -f "$PREFIX/eurokey-ibus" "$PREFIX/logo.jpg"
rmdir "$PREFIX" 2>/dev/null || true
rm -f "$COMPONENT_DIR/eurokey.xml"

echo "==> removed. Restart ibus:  ibus restart"
echo "Config kept at ${XDG_CONFIG_HOME:-$HOME/.config}/eurokey/"
