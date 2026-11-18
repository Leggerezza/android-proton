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

### Added (P1)

- **P1**: Go core ported to Android — `proton`, `stealth`, `tunnel`, `socks5`, `store`
  and the gomobile contract layer `protonnode` (Init/Login/Start/Stop/Status/RecentLogs).
- **P1**: real Proton Stealth tunnel verified on Windows and on the OnePlus Pad 4
  (WireGuard handshake 250 ms, in-tunnel `api.ipify.org` returns a Proton exit IP).
- **P1**: gomobile AAR (`scripts/build-core-aar.sh`) with 16 KB-aligned `libgojni.so`,
  wired into the app as `app/libs/core.aar`.
- **P1**: device instrumentation smoke test proving the JNI bridge loads the Go runtime.
- **P1**: `protonnode-cli` verification harness (convert/selftest/run/state).

### Changed

- CI: JDK 25 + Go 1.27.4, ktlint/Android Lint gate, release-APK path fixed for
  the 16 KB check; Go tests run with `-race`.
- **P1**: `scripts/check-16kb.sh` now extracts every `.so` by exact entry name and fails
  loudly instead of silently passing when extraction fails (Git Bash + Info-ZIP).
- **P1**: SOCKS5 no longer logs destination hosts (privacy, PROMPT.md 0.5).
