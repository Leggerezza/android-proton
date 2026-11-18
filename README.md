# android-proton

Android 16 (API 36) client that turns a Proton Stealth tunnel (WireGuard over TLS)
into a local SOCKS5 node (`127.0.0.1:1080`) for mihomo / FlClash.

- Target: OnePlus Pad 4 (OPD2514), OxygenOS 16, arm64-v8a, 16 KB pages
- Package: `io.github.dsh.protonnode` (debug builds install as `io.github.dsh.protonnode.debug`)
- Scope: SOCKS5 node only (no VpnService); routing is done by FlClash
- Status: **P1 done** — the Go core opens a real Proton Stealth tunnel and serves
  SOCKS5 TCP on a real account, on Windows and on the device. No UI yet (the
  phase gate requires P2 before UI work). UDP, scanning and the foreground
  service are the next phases; see `docs/TEST-EVIDENCE.md`.

## Layout

| Path | What |
|---|---|
| `core/` | Go module: `proton` (API/SRP/cert), `stealth` (TunSafe frames + TLS bind), `tunnel` (wireguard-go + gVisor netstack), `socks5` (TCP), `store` (non-secret cache), `protonnode` (gomobile contract) |
| `core/cmd/protonnode-cli` | verification harness: `convert` / `selftest` / `run` / `state` |
| `app/` | Android application (no UI yet); links the gomobile AAR in `app/libs/core.aar` |
| `docs/` | architecture, protocol, P0 audit, per-phase test evidence |

## Build

Prerequisites: JDK 17+ (verified on 25), Android SDK 36, Go 1.24+ (verified on
1.27.4), NDK r28+ for the core AAR. `local.properties` must point at the SDK
(`sdk.dir=...`); it is git-ignored.

```bash
# 1. Go core: tests
cd core && go vet ./... && go test ./... -count=1

# 2. Bind the core into an AAR (needs NDK + gomobile, see the script header)
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/28.2.13676358
scripts/build-core-aar.sh                 # -> app/libs/core.aar (git-ignored)

# 3. Android app
./gradlew assembleDebug                   # -> app/build/outputs/apk/debug/app-debug.apk
./gradlew :app:ktlintCheck :app:lintDebug :app:testDebugUnitTest
./gradlew :app:connectedDebugAndroidTest   # JNI smoke test, needs a device

# 4. Release APK + 16 KB gate (checks every .so in the APK)
./gradlew :app:assembleRelease
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release-unsigned.apk
```

## Verifying a real tunnel (P1 evidence)

`core/protonnode` never writes credentials; the session JSON is produced by SRP login
or by importing an existing Windows session. The CLI harness takes that JSON:

```bash
# desktop
go build -o protonnode-cli ./core/cmd/protonnode-cli
./protonnode-cli convert -from <windows state.json> -out session.json
./protonnode-cli selftest -state session.json -cfg ./cfg -socks 11080 -mode normal

# device (cross-compiled, no cgo: Android has no /etc/resolv.conf for Go's pure resolver)
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o cli ./core/cmd/protonnode-cli
adb push cli /data/local/tmp/p1/protonnode-cli
adb shell /data/local/tmp/p1/protonnode-cli selftest -state /data/local/tmp/p1/session.json \
    -cfg /data/local/tmp/p1/cfg -dns <dns-server-of-the-wifi> -mode normal
```

A pass prints `tunnel up in …ms` and an exit IP that is **not** your own.

## Docs

| File | Contents |
|---|---|
| `docs/ARCHITECTURE.md` | Components, state machine, DNS roles, loop guard |
| `docs/PROTOCOL.md` | Verified Proton Stealth facts, frame format, Android re-verification |
| `docs/P0-AUDIT.md` | Dependency audit of the Windows baseline → Android verdicts, assumptions A1–A13 |
| `docs/TEST-EVIDENCE.md` | Per-phase commands and raw output (P0 + P1) |
| `docs/RELEASE-CHECKLIST.md` | Mandatory release gates (16 KB runtime, signing, SBOM, secrets, CVE) |

## Known limitations (apply to the finished client, stated up front)

- Stealth carries WireGuard inside **TLS over TCP**: packet loss becomes TCP
  retransmission and head-of-line blocking. It buys **reachability**, not low
  latency — it is not a competitive-gaming transport.
- Proton exit NAT type is not guaranteed; Steam P2P may not work.
- Wine/Box64 may spawn helper processes with a different UID; split-tunnelling
  rules must be verified per game, not assumed.
- P1 serves **TCP only**: SOCKS5 UDP ASSOCIATE answers 0x07 until P2.

License: GPL-3.0-or-later (derived from Proton's open-source protocol code).
