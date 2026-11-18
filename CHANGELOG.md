# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/), generated with git-cliff.

## [Unreleased]

### Added

- Repository skeleton: CI, security workflows, protocol docs, 16 KB check.
- **P0**: dependency audit of `baseline/proton-sidecar` (`docs/P0-AUDIT.md`).
- **P0**: Gradle skeleton (AGP 9.5.1 / Gradle 9.6.0 / Kotlin via built-in Kotlin
  support, ktlint 14.2.0) and the `app` module; `./gradlew assembleDebug` builds an APK.
- **P0**: `core/go.mod` so `cd core && go test ./...` is a real gate.
- **P0**: `docs/TEST-EVIDENCE.md` with the raw output of every P0 check.
- **P0**: `CODE_OF_CONDUCT.md` (required by the repo checklist).

### Changed

- CI: JDK 25 + Go 1.27.4, ktlint/Android Lint gate, release-APK path fixed for
  the 16 KB check.
