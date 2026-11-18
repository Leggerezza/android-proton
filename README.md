# android-proton

Android 16 (API 36) client that turns a Proton Stealth tunnel (WireGuard over TLS)
into a local SOCKS5 node (`127.0.0.1:1080`) for mihomo / FlClash.

- Target: OnePlus Pad 4 (OPD2514), OxygenOS 16, arm64-v8a, 16 KB pages
- Package: `io.github.dsh.protonnode` (debug builds install as `io.github.dsh.protonnode.debug`)
- Scope: SOCKS5 node only (no VpnService); routing is done by FlClash
- Status: **P0 done** (audit + buildable skeleton). No tunnel, no UI yet — see
  `docs/P0-AUDIT.md` for what comes next and `docs/TEST-EVIDENCE.md` for proof.

## Build

Prerequisites: JDK 17+ (verified on 25), Android SDK 36, Go 1.24+ (verified on
1.27.4). `local.properties` must point at the SDK (`sdk.dir=...`); it is git-ignored.

```bash
# Android: debug APK
./gradlew assembleDebug          # -> app/build/outputs/apk/debug/app-debug.apk
./gradlew :app:ktlintCheck :app:lintDebug :app:testDebugUnitTest

# Go core (module only in P0)
cd core && go vet ./... && go test ./...

# Release APK + 16 KB gate
./gradlew :app:assembleRelease
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release-unsigned.apk
```

Toolchain versions used for the P0 evidence (see `docs/TEST-EVIDENCE.md`):
Gradle 9.6.0 (wrapper-pinned), AGP 9.5.1, Kotlin through AGP's built-in Kotlin
support, ktlint 14.2.0, Go 1.27.4, JDK 25.0.3, Android SDK 36.1.0.

## Docs

| File | Contents |
|---|---|
| `docs/ARCHITECTURE.md` | Components, state machine, DNS roles, loop guard |
| `docs/PROTOCOL.md` | Verified Proton Stealth facts and frame format |
| `docs/P0-AUDIT.md` | Dependency audit of the Windows baseline → Android verdicts |
| `docs/TEST-EVIDENCE.md` | Per-phase commands and raw output |

## Known limitations (apply to the finished client, stated up front)

- Stealth carries WireGuard inside **TLS over TCP**: packet loss becomes TCP
  retransmission and head-of-line blocking. It buys **reachability**, not low
  latency — it is not a competitive-gaming transport.
- Proton exit NAT type is not guaranteed; Steam P2P may not work.
- Wine/Box64 may spawn helper processes with a different UID; split-tunnelling
  rules must be verified per game, not assumed.

License: GPL-3.0-or-later (derived from Proton's open-source protocol code).
