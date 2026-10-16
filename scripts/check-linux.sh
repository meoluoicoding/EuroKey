#!/bin/sh
# EuroKey Linux (IBus) checks. Run on Linux (Ubuntu); cross-compiling from
# other hosts only proves the port compiles, not that IBus works.
#
#   sh scripts/check-linux.sh
set -eu

HERE=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$HERE/linux"

echo "== go vet =="
go vet ./...

echo "== go test =="
go test ./...

echo "== build (amd64 + arm64) =="
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/eurokey-ibus ./cmd/eurokey-ibus
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/eurokey-ibus-arm64 ./cmd/eurokey-ibus

echo "PASS: linux checks OK"
