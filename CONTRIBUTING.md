# Contributing

## Setup

1. JDK 17+ (CI and the P0 evidence use 25), Android SDK 36, NDK r28+ (from P1 on), Go 1.24+ (P0 evidence: 1.27.4).
2. Create `local.properties` with `sdk.dir=/path/to/Android/Sdk` (git-ignored).
3. Optional hooks: `lefthook install` (or `git config core.hooksPath .githooks`).
4. Sanity check:

```bash
./gradlew :app:assembleDebug :app:ktlintCheck :app:lintDebug :app:testDebugUnitTest
cd core && go vet ./... && go test ./...
```

## Rules

- Conventional Commits (`feat(scan): ...`, `fix(socks5): ...`); scopes are listed in `.commitlintrc.json`.
- Every PR: CI green, no new lint errors, no secrets (gitleaks).
- Protocol changes require a test vector in `core/testdata/` and an entry in `docs/PROTOCOL.md`.
- Never commit keystores, tokens, or generated APKs.
- No fake results: every "pass" in `docs/TEST-EVIDENCE.md` carries the command and its raw output.
- Before recording any **network** result, prove the path is direct: `protonnode-cli netcheck -target <entry>:443`
  must show the physical interface IP (not 127.0.0.1, not Tailscale 100.x, not a TUN). The dev machine runs
  FlClash in system-proxy mode, which Go ignores, but a TUN would still capture us.

## Phase gates

P1 (real tunnel) and P2 (SOCKS5 TCP/UDP) must pass before UI work starts.

```
P0 audit+skeleton -> P1 real tunnel -> P2 SOCKS5 TCP+UDP -> P3 scan
  -> P4 foreground service -> P5 UI -> P6 FlClash -> P7 release -> P8 hardening
```
