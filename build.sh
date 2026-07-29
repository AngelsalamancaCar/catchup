#!/usr/bin/env bash
# Compila projectmapper para windows/amd64, darwin/arm64 y linux/amd64 —
# §3 Fase 4, tarea 5. Cada binario es standalone (stdlib + excelize, sin
# CGO), listo para copiar junto a data/ sin instalar Go en destino.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

VERSION="${1:-dev}"
OUT_DIR="dist"
LDFLAGS="-s -w"

targets=(
  "windows amd64 .exe"
  "darwin arm64 "
  "linux amd64 "
)

mkdir -p "$OUT_DIR"

for target in "${targets[@]}"; do
  read -r goos goarch ext <<< "$target"
  out="$OUT_DIR/projectmapper-${goos}-${goarch}${ext}"
  echo "==> building $out"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/projectmapper
done

echo "listo. Binarios en $OUT_DIR/ (versión: $VERSION)"
