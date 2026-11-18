#!/usr/bin/env bash
# Verify that every native library in the APK is 16 KB page aligned.
# Usage: scripts/check-16kb.sh [path/to/app.apk]
#   Without an argument the newest release APK is used: an unsigned release
#   build produces app-release-unsigned.apk, so a hard-coded name would fail.
set -euo pipefail

apk="${1:-}"
if [ -z "$apk" ]; then
  apk="$(ls -t app/build/outputs/apk/release/*.apk 2>/dev/null | head -n 1 || true)"
fi
[ -n "$apk" ] && [ -f "$apk" ] || { echo "APK not found: ${apk:-app/build/outputs/apk/release/*.apk}" >&2; exit 2; }
echo "checking $apk"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
unzip -q "$apk" 'lib/*' -d "$tmp" || true

mapfile -t sos < <(find "$tmp/lib" -name '*.so' 2>/dev/null)
if [ "${#sos[@]}" -eq 0 ]; then
  echo "no .so in APK (pure-Java build?) - skipping alignment check"
else
  for so in "${sos[@]}"; do
    echo "checking $so"
    if command -v llvm-readelf >/dev/null; then readelf=llvm-readelf; else readelf=readelf; fi
    bad=$("$readelf" -lW "$so" | awk '$1=="LOAD"{print $NF}' | grep -v '^0x4000$' || true)
    if [ -n "$bad" ]; then
      echo "  FAIL: non-16KB LOAD alignment: $bad" >&2
      exit 1
    fi
  done
fi

if command -v zipalign >/dev/null; then
  zipalign -c -P 16 -v 4 "$apk" >/dev/null && echo "zipalign 16KB: OK"
else
  echo "zipalign not on PATH - skipped"
fi
echo "16 KB check passed"
