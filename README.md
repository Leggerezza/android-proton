# android-proton

Android 16 (API 36) client that turns a Proton Stealth tunnel (WireGuard over TLS)
into a local SOCKS5 node (`127.0.0.1:1080`) for mihomo / FlClash.

- Target: OnePlus Pad 4 (OPD2514), OxygenOS 16, arm64-v8a, 16 KB pages
- Scope: SOCKS5 node only (no VpnService); routing is done by FlClash
- Status: **skeleton** — see `docs/` and `../ANDROID16-PROTON-CLIENT-PROMPT.md`

## Build

```bash
./gradlew :app:assembleRelease
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release.apk
```

License: GPL-3.0-or-later (derived from Proton's open-source protocol code).
