#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/bin"

mkdir -p "$BIN"

echo "Building openslackd..."
go build -o "$BIN/openslackd" "$ROOT/cmd/openslackd"

echo "Building openslackctl..."
go build -o "$BIN/openslackctl" "$ROOT/cmd/openslackctl"

echo "Done. Binaries in $BIN/"
ls -lh "$BIN"/
