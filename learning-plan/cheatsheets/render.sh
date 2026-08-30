#!/usr/bin/env bash
# Render cheatsheet Markdown to PDF.
#
#   ./render.sh            # every *.md in this folder (skips README.md)
#   ./render.sh 06-functions.md 19-stdlib.md
#
# Pipeline: main.go turns the Markdown into print-styled HTML, then headless
# Chrome prints it to pdf/<name>.pdf. Needs only Go and Google Chrome.
set -euo pipefail

cd "$(dirname "$0")"

CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
[ -x "$CHROME" ] || { echo "Chrome not found at: $CHROME (override with \$CHROME)" >&2; exit 1; }

mkdir -p pdf
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# tools/render is its own tiny module, so build it from inside its directory.
( cd tools/render && go build -o "$tmp/render" . )

files=("$@")
if [ ${#files[@]} -eq 0 ]; then
  files=()
  for f in *.md; do
    [ "$f" = "README.md" ] || files+=("$f")
  done
fi

for md in "${files[@]}"; do
  name="$(basename "$md" .md)"
  "$tmp/render" "$md" "$tmp/$name.html"
  "$CHROME" --headless --disable-gpu --no-pdf-header-footer \
    --print-to-pdf="pdf/$name.pdf" "$tmp/$name.html" 2>/dev/null
  echo "pdf/$name.pdf  <-  $md"
done
