#!/usr/bin/env bash
# Build the Go core into an Android AAR with gomobile.
#
#   scripts/build-core-aar.sh            -> app/libs/core.aar (arm64-v8a, API 26)
#
# Requirements (all pinned by the P1 evidence):
#   Go 1.27.4 with the tool directive on golang.org/x/mobile/cmd/gobind
#   (go get -tool golang.org/x/mobile/cmd/gobind)
#   gomobile      (go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init)
#   Android NDK   r28+ (ANDROID_NDK_HOME), JDK 17+ (javac on PATH)
#
# The output is a build artifact and is git-ignored; a fresh checkout builds
# the app without it, and this script adds the real core.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$root/app/libs/core.aar}"
: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to the NDK (r28 or newer)}"

mkdir -p "$(dirname "$out")"
echo "building $out"
cd "$root/core"
gomobile bind -target=android/arm64 -androidapi 26 -o "$out" ./protonnode
ls -l "$out"
echo "check the 16 KB alignment with: scripts/check-16kb.sh <apk>"
