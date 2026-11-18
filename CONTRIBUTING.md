# Contributing

## Setup

1. JDK 17, Android SDK 36, NDK r28+, Go 1.24+.
2. `git config core.hooksPath .githooks` (or install `lefthook`).
3. `./gradlew :app:assembleDebug` and `cd core && go test ./...`.

## Rules

- Conventional Commits (`feat(scan): ...`, `fix(socks5): ...`).
- Every PR: CI green, no new lint errors, no secrets (gitleaks).
- Protocol changes require a test vector in `core/testdata/` and an entry in
  `docs/PROTOCOL.md`.
- Never commit keystores, tokens, or generated APKs.

## Phase gates

P1 (real tunnel) and P2 (SOCKS5 TCP/UDP) must pass before UI work starts.
