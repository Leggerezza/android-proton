# Architecture

## Components

```
Compose UI (P5) -> Foreground Service (P4) -> protonnode (gomobile contract)
                                             |
                                             +- proton     (SRP login, logicals, certificate, tokens)
                                             +- stealth    (TunSafe frames + TLS-over-TCP bind)
                                             +- tunnel     (wireguard-go + gVisor netstack, no TUN)
                                             +- socks5     (TCP CONNECT + UDP ASSOCIATE)
                                             +- store      (non-secret cache: servers, prefs)
```

The Go package `core/protonnode` is the only surface the app sees (gomobile: strings and
errors, everything structured is JSON). Credentials never reach the Go disk layer: the app
encrypts `SessionJSON()` with an Android Keystore key and injects it back with
`SetSession()`. `store` only holds the logical server list and preferences.

## State machine (P1, implemented)

```
idle --Start()--> scanning --candidates ok--> connecting --handshake ok--> connected
  ^                   |                           |                          |
  |                   v                           v                          | 2x health fail
  +---------------- error <---- reconnect (60s) <----------------------- reconnecting
```

- `scanning`: server list (API, else cache) + selection + L1 TCP probe of entry IPs
  (12 concurrent, deduplicated by IP, max 3 survivors).
- `connecting`: certificate reuse/new, then TLS + WireGuard handshake with a 25 s budget,
  falling through the probed candidates.
- `connected`: SOCKS5 on `127.0.0.1:<socksPort>`; a health probe runs every 30 s and
  two consecutive failures drop the tunnel.
- `reconnecting`: 60 s backoff, then selection runs again. With `lockServer` (default on
  in game mode) the same logical server is retried for up to 3 rounds before switching, because
  a server switch changes the exit IP and kills every game session (PROMPT.md 7).

`Status()` is an atomic snapshot of this machine plus the WireGuard counters; it never
touches the network.

## DNS roles (never mix)

| Role | Resolver | Path |
|---|---|---|
| mihomo rule matching | mihomo fake-ip | local |
| SOCKS5 destination | 10.2.0.1 inside tunnel | tunnel |
| Proton API | Go, pinned IP/DNS | direct, strict TLS |
| Scan targets | none | IP literals only |

Android has no `/etc/resolv.conf`: a pure-Go binary cannot use the system resolver, so the API
client can pin a DNS server (`SetAPIDNS`). The AAR built by gomobile is a cgo build and uses
bionic's `getaddrinfo` normally.

## Loop guard

With FlClash TUN enabled the app's own TLS would be captured and routed back
into its own SOCKS port. The mihomo profile must contain
`PROCESS-NAME,<app package>,DIRECT`; the app itself never opens a VpnService.

## UDP flow (P2)

```
mihomo --UDP--> 127.0.0.1:<relay port>     (one socket per association)
                     |
                     +- dst -> netstack UDPConn   (LRU 64, idle 120s)
                                 |
                                 +- WireGuard data frames -> TLS -> Proton exit
```

The relay owns one UDP socket per association and one connected netstack socket per
destination. A destination that stays idle for 120s is recycled, which keeps the
source port stable for a game that keeps talking to the same server.
