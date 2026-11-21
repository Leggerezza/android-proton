#!/bin/bash
# F-Droid build recipe entry point (kept next to metadata.yml so the recipe can be
# read without the whole checkout).
#
# Requirements that the build environment must satisfy:
#   * Go >= 1.26 (go.mod uses a tool directive); if the image ships an older Go,
#     the F-Droid recipe needs a newer base image or a go.mod downgrade.
#   * Android NDK r28 (metadata pins 28.2.13676358) and JDK 17.
set -euo pipefail

export GOPATH="${GOPATH:-$HOME/go}"
export PATH="$GOPATH/bin:$PATH"
export ANDROID_NDK_HOME="$NDK"

echo "go: $(go version)"

# gomobile bind needs the gobind tool; the version is pinned in go.mod's tool
# directive, but installing it explicitly keeps the recipe independent of it.
go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20261117202452-47699113848a
gomobile init

scripts/build-core-aar.sh
./gradlew --no-daemon assembleRelease

ls -l app/build/outputs/apk/release/
