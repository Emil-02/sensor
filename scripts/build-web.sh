#!/usr/bin/env bash
# Builds the static web app into dist/ (or $1): Go WASM + pinned, self-hosted
# OCR assets so the page never loads code or data from third-party CDNs.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root/dist}"

rm -rf "$out"
mkdir -p "$out/vendor/tesseract" "$out/vendor/core" "$out/lang"

cp "$root"/web/index.html "$root"/web/app.js "$root"/web/style.css "$root"/web/favicon.svg "$out/"
touch "$out/.nojekyll"

cd "$root"
GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o "$out/sensor.wasm" ./cmd/sensor-wasm
goroot="$(go env GOROOT)"
if [[ -f "$goroot/lib/wasm/wasm_exec.js" ]]; then
  cp "$goroot/lib/wasm/wasm_exec.js" "$out/"
else
  cp "$goroot/misc/wasm/wasm_exec.js" "$out/"
fi

(cd "$root/web" && npm ci --ignore-scripts --no-audit --no-fund --silent)
nm="$root/web/node_modules"
cp "$nm/tesseract.js/dist/tesseract.min.js" "$nm/tesseract.js/dist/worker.min.js" "$nm/tesseract.js/LICENSE.md" "$out/vendor/tesseract/"
# Only the LSTM cores are needed (OEM 1, no legacy engine).
cp "$nm"/tesseract.js-core/tesseract-core*-lstm.wasm.js "$nm/tesseract.js-core/LICENSE" "$out/vendor/core/"
cp "$nm/@tesseract.js-data/ind/4.0.0_best_int/ind.traineddata.gz" "$out/lang/"

echo "web app built in $out"
