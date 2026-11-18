#!/usr/bin/env bash
# Minimal CycloneDX-ish SBOM: Go modules (Gradle deps added in CI).
set -euo pipefail
echo '{"bomFormat":"CycloneDX","specVersion":"1.5","components":['
first=1
if [ -f core/go.sum ]; then
  awk '{print $1"@"$2}' core/go.sum | sort -u | while read -r m; do
    if [ "$first" -eq 1 ]; then first=0; else echo ","; fi
    printf '{"type":"library","name":"%s"}' "$m"
  done
fi
echo ']}'
