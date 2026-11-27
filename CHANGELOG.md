# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/), generated with git-cliff.

## 0.2.0 — 2026-11-27

- **Fast first connect**: the node now probes the pool over plain TCP and connects to the first
  entry under 700 ms immediately (then verifies it with the WireGuard handshake); the full L2/L3
  scan only runs when no quick entry answers or the quick ones fail.
- **Certificate key rotation**: Proton answers 409/2500 ("client public key fingerprint
  conflict") when the stored Ed25519 key is already registered elsewhere; the node now rotates the
  seed and retries the certificate request once instead of retrying forever.
- **Saved logs keep their name**: MediaStore was appending ".txt" to the text/plain download;
  the save path now uses an opaque MIME type, so the file stays "proton-node-….log".

## 0.1.9 — 2026-11-27

- **Settings is now a root list with real sub-pages**: Account, Theme & colors, Language,
  Notifications, Behavior, FlClash and About each open their own page with a back arrow, and the
  root row shows the current value (session, theme, language, notification state, mode/transport,
  version). The single scrolling page and the jump bar are gone; a layout that broke twice is
  replaced by separate composables, verified on the emulator.

## 0.1.8 — 2026-11-25

- **Settings navigation**: a category bar (Account / Theme & colours / Language / Notifications /
  Behavior / FlClash / About) jumps to the matching section. Sections record their own offsets,
  so the bar never assumes a layout. The real sub-page split is still pending - an earlier
  attempt broke the layout and was reverted.

## 0.1.7 — 2026-11-25

- **Latency measurement**: Servers gained a "Measure latency" action that runs the L1 TCP probe
  over the current list (bounded, background) and merges the round trip into every row, so a
  server no longer shows a blank because it was never part of an L3 scan. The list marks the
  source: ✓ verified by a WireGuard handshake, ~ TCP round trip only, — never measured.
- **Country view**: one row per exit country with a flag, server count and the fastest measured
  latency, sorted by it; unmeasured countries say "not measured".
- **Log noise**: SOCKS handshake/bad-request failures are aggregated per second per kind, like
  the CONNECT failures, so a client that opens hundreds of doomed connections cannot flood the
  ring log.

## 0.1.6 — 2026-11-25

- **Log report**: Share now packages the ring log into a real `proton-node-<stamp>.log` file with a
  diagnostic header (build, device, Android, timezone, network, session, tunnel state and
  settings) before handing it to the chooser; Save writes the same file to Download via
  MediaStore, and Copy keeps the quick path.
- **Settings**: the session banner (expired / signed in / signed out) sits at the top, Account
  moved next to it, and Notifications and About are their own sections.
- **Servers**: a "By country" view ranks countries by their fastest measured entry, shows a flag
  and the server count per country, and prints "not measured" instead of a blank latency.

## 0.1.5 — 2026-11-25

- **Reconnect**: a tunnel that had been up now retries immediately with the entries that just
  worked, and only repeated failures back off (2s, 5s, 15s, 30s, 60s). The health check runs
  every 15s, so a dead tunnel is noticed within 30s instead of a minute.
- **Notifications**: POST_NOTIFICATIONS is requested once the first-run notice is accepted, and
  Settings shows whether notifications are allowed with a shortcut to the system page.
- **Logs**: every line carries INFO/WARN/ERROR (derived from what it says), the list opens at
  the newest line, levels can be filtered, and the export button hands the redacted text to any
  app. The FLAG_SECURE screenshot block is gone.
- **Scan**: each verified entry has a "Use" action that pins that server and connects.
- **DNS**: a failed in-tunnel lookup is retried once; identical CONNECT failures are folded into
  one log line per second instead of flooding the ring log.
- Home: Disconnect is an outlined button (it was styled as the primary action).

## 0.1.4 — 2026-11-25

- Diagnostics from a real flaky network (a log showed the API returning EOF while every L2
  TLS handshake failed, then recovering): the scan summary now names the failure classes
  (`l2 0/24 tls ok (eof 24)`), transport errors on the primary API host fall back once to
  vpn-api.proton.me, and the status line shows the SNI that was actually used again.

## 0.1.3 — 2026-11-22

- **WireGuard/UDP transport**: settings can switch the outer transport between Stealth
  (WireGuard in TLS/TCP) and plain WireGuard over UDP, which is far faster where UDP is
  allowed; the UDP port comes from /vpn/v2/clientconfig (fallback 51820).
- **IPv6**: the tunnel can carry Proton's fixed IPv6 address and DNS
  (2a07:b944::2:2 / ::2:1), adds \`::/0\` to the peer and only enables it on servers that
  advertise the IPv6 feature (bit 16), matching the official client.
- **Excluded countries**: a picker lists every exit country Proton offers (localised names)
  and servers in the selected countries are never connected, even when pinned.
- **First-run notice**: the app now states it is an unofficial third-party client without
  VPN servers of its own; Continue unlocks after three seconds and the notice never returns.

## 0.1.2 — 2026-11-21

- Tunnel MTU is now a setting (1200 / 1320 / 1420, default 1320) and is passed through
  the start config, so a TCP-carrier stall can be compared without a rebuild.
- SOCKS CONNECT failures now log the address family and error class (never the destination),
  which tells an IPv6 target with no route apart from a DNS failure.
- The generated mihomo profile sets `dns.ipv6: false` so apps stop receiving AAAA answers
  the tunnel cannot route.

- Latency in the server list (measured by the last scan), the current server marked, and a
  "By latency" sort; the home screen now shows live ↓/↑ rates instead of only totals.
- In-tunnel DNS answers are cached for 60 s: a music player opening many connections to the
  same hosts no longer pays a DNS round trip each time.

- Region pinning: `US-CA` / `US-WA` in the default-country field selects a region by
  server-name prefix, so a US exit can be the west coast instead of US-CO/US-NY.

Latency: normal mode no longer pins the country order to `US, CA, NL` (that put an Asia user
on a US exit, 1.2–2.2 s per request). Automatic selection now scans a country-diverse pool
ordered by a cheap TCP RTT pre-pass, so the nearest entry wins instead of the one with the
best load score. Game mode keeps its JP/HK/KR/SG/US shortlist but also measures RTT first.
## 0.1.1 — 2026-11-21

First public build: the identity moved to `io.github.leggerezza.protonnode`, the repository is
public, and CI builds the Go core before Gradle. govulncheck findings fixed (brotli,
klauspost/compress, circl); gitleaks allowlists the public server keys quoted as evidence.
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

### Added (P2)

- **P2**: SOCKS5 UDP ASSOCIATE - loopback relay, per-association dst->socket LRU (64)
  with a 120s idle recycle, FRAG!=0 dropped, all three ATYP, in-tunnel DNS for
  domains, lifetime bound to the TCP control connection.
- **P2**: tunnel DialUDP / ResolveHost (netstack UDP); Status().udp is now true.
- **P2**: protonnode-cli udptest (DNS query over SOCKS5 UDP, 3 attempts).
- **P2**: tests for UDP framing, echo, 16 concurrent targets, LRU/idle, teardown,
  netstack UDP through the tunnel and TCP half-close.

### Verified

- **A12**: the Go core runs on a real 16 KB page-size Android 16 guest (emulator,
  `PAGE_SIZE=16384`): JNI smoke OK(1 test), socks5 11/11, tunnel (TLS+WireGuard+netstack+UDP
+half-close) 3/3. The arm64 APK cannot run on the x86_64 emulator (its arm64 binary
  translation aborts), so A12 was checked with `-PemulatorAbi=x86_64`; the arm64 16 KB
  confirmation stays a P7 item on a 16 KB device.
- **protonnode-cli** now writes rotated tokens back to `-state` (Proton refresh tokens are
  single-use; not persisting them made the next run fail with 401/2001).

### Added (P3)

- **P3**: three-level server scan - L1 TCP, L2 real TLS (utls Chrome + rotated safe SNI),
  L3 complete WireGuard handshake; only L3 counts for automatic selection.
- **P3**: per-network result cache (fingerprint + level, TTL 30 min) in `scan-cache.json`,
  cache-hit scans return in 0 ms with `cacheHit: true`.
- **P3**: `StartScan` / `ScanProgress` / `ScanResults` / `CancelScan` / `LastScanSummary`
  (cancel measured at 1 ms) and `SetNetworkFingerprint`.
- **P3**: `docs/schemas/scan-{results,progress}.schema.json` plus a conformance test.
- **P3**: `protonnode-cli scan` command (`-scope`, `-network`, `-cancel-after`).

### Added (P4)

- **P4**: `ProtonService` specialUse foreground service (ongoing silent notification with a
  Stop action, 5s status refresh, `onDestroy` always stops the node within 5s).
- **P4**: `NetworkMonitor` - network changes update the scan fingerprint and restart the
  tunnel immediately instead of waiting for the 60s backoff.
- **P4**: `SessionStore` with Android Keystore AES-256-GCM for the session JSON;
  `CoreController` as the single serialised bridge to gomobile.
- **P4**: `BootReceiver` (opt-in auto start) and `HealthWorker` (WorkManager, best effort).
- **P4**: JVM unit tests for the notification text and the network fingerprint.

### Added (P5)

- **P5**: Compose UI with a navigation rail (>= 600dp) or bottom bar (< 600dp), a 840dp
  expanded layout on the tablet, Material You colors and a system/light/dark theme.
- **P5**: Home (state, connect/disconnect, server/traffic/uptime/error), Servers (mode + search),
  Scan (scope, progress, L3 results), Settings (theme, language, mode, autostart, API DNS,
  account login/logout, export/import, battery optimization) and Logs behind FLAG_SECURE.
- **P5**: per-app language (en / zh) via AppCompatDelegate, `values-zh` strings.
- **P5**: `ConfigCodec` export/import with a SHA-256 checksum (no credentials) + unit tests.
- **P5**: the app now persists rotated Proton session tokens (`CoreController.persistSession`,
  refreshed by the foreground service) — the P2 write-back bug no longer bites on device.
- **P5**: the service ignores repeated network callbacks with the same fingerprint.

### Fixed (P5)

- The core never created its ring log, so `RecentLogs()` was empty in the app; `Init` now
  creates it (500 lines) and `RecentLogs` is nil-safe, with a regression test.
- The server list stayed empty because the ViewModel loaded it before the session was
  restored; the Servers screen now loads on entry.
- Game mode listed Secure Core servers (PROMPT.md 7 forbids it).

### Fixed (P5, login UX)

- The account section now reports the session verdict (`authState` unknown/valid/expired), so
  a stored-but-dead session is visible instead of silently looking signed in.
- Sign in never hits Proton when a usable session exists: `decideLogin` short-circuits to
  "already signed in" and no request is sent.
- Clear feedback and a cooldown: the button shows signing-in/success/failure/"try again in Ns",
  and `LoginGate` backs off 30s (120s when rate limited, exponential up to 10 min) so repeated
  taps cannot trigger Proton's "Too many requests".

- **P6 groundwork**: a "Lock server during game" switch (default on) that is passed to the core
  as `lockServer`, and the account status now prints the verification time in local time.

### Added (P6)

- **P6**: `FlClashConfig` generates the mihomo profile (socks5 127.0.0.1:1080 udp:true, GAME/PROXY
  groups, fake-ip DNS, TUN) and validates it (loop guard first, MATCH order, udp).
- **P6**: Settings can copy the profile and check the one currently in the clipboard.
- **P6**: verified end to end on the tablet with FlClash 0.9.4: this app's own traffic is
  DIRECT in FlClash while other apps go through `proton-node`.

- **P6**: the generated FlClash profile now keeps domestic traffic domestic (WeChat/Alipay/Taobao
  domain suffixes + `GEOSITE,cn`/`GEOIP,CN` before `MATCH`); proxying every app through Proton was
  slow and tripped WeChat's risk control. The validator rejects profiles without the CN rules.

- **P6**: manual server/country selection (Servers screen pin + country chips + Auto). A pinned
  server bypasses the L3 cache and fails loudly instead of falling back to a bare TCP probe.
- **P6**: the generated profile now mirrors the Windows rules: fake-ip excludes the Proton API
  domains, foreign domain-suffixes sit above GEOSITE,cn, GEOIP,CN uses no-resolve, and
  `find-process-mode: strict` / `profile.store-selected` are set.

- **P6**: the profile's TUN MTU is 9000 instead of 1280. With TUN on, the node's handshake
  succeeds but its data path returns `rep 0x4`; without TUN it connects fine, so the MTU
  mismatch (tunnel 1320 vs TUN 1280) is the prime suspect. Verification is pending.

- **P6**: retested under FlClash TUN: the node connects and real app traffic (NouTube, Play
  Services) flows through `proton-node` while this app stays DIRECT. The earlier `rep 0x4` storm
  was a bad exit, not only MTU; throughput tuning is tracked for P7.

- **P6**: `Default country` setting: the user types any two-letter code (case-insensitive,
  validated, empty = automatic), which is stored and shown as the current selection; a pinned
  server and a pinned country stay mutually exclusive.

- **P6**: server features are now surfaced (`secureCore/p2p/tor/streaming`), shown as badges in
  the server list and filterable (P2P 1971, Tor 7 of 2784); game mode excludes Secure Core and Tor.

### Security

- Dependencies bumped past four advisories found by govulncheck (brotli 1.2.7,
  klauspost/compress 1.20.2, cloudflare/circl 1.6.3); `govulncheck` is clean now.
- `.gitleaks.toml` allowlists the public Proton server keys quoted in the scan evidence
  (the generic api-key heuristic was flagging them); gitleaks runs pinned at v8.30.1.

### Changed (release identity)

- The application id and Kotlin namespace are now `io.github.leggerezza.protonnode`
  (GitHub: [Leggerezza](https://github.com/Leggerezza)); the Go module moved to
  `github.com/Leggerezza/android-proton/core`.
- Every Go and Kotlin source file carries an `SPDX-License-Identifier: GPL-3.0-or-later`
  header; README gained a license/provenance section.
- `metadata/io.github.leggerezza.protonnode/` holds the draft F-Droid recipe
  (`metadata.yml`, `build.sh`, `LICENSE`, NonFreeNet anti-feature).

### Changed

- CI: JDK 25 + Go 1.27.4, ktlint/Android Lint gate, release-APK path fixed for
  the 16 KB check; Go tests run with `-race`.
- **P1**: `scripts/check-16kb.sh` now extracts every `.so` by exact entry name and fails
  loudly instead of silently passing when extraction fails (Git Bash + Info-ZIP).
- **P1**: SOCKS5 no longer logs destination hosts (privacy, PROMPT.md 0.5).
- **P2**: SOCKS5 success replies now carry the real BND for UDP sockets (UDP ASSOCIATE
  used to answer 0.0.0.0:0).
