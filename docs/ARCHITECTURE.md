# Architecture

## Components

```
Compose UI -> Foreground Service -> CoreBridge (gomobile)
                                     |
                                     +- proton  (auth, logicals, cert)
                                     +- stealth (TLS bind + TunSafe frames)
                                     +- tunnel  (wireguard-go + netstack)
                                     +- socks5  (TCP CONNECT + UDP ASSOCIATE)
```

## State machine

```
idle --start--> scanning --ok--> connecting --ok--> connected
  ^                | fail                          | probe x2 fail
  |                v                               v
  +------------ error <---- 60s <------------ reconnecting
```

## DNS roles (never mix)

| Role | Resolver | Path |
|---|---|---|
| mihomo rule matching | mihomo fake-ip | local |
| SOCKS5 destination | 10.2.0.1 inside tunnel | tunnel |
| Proton API | Go, pinned IP | direct, strict TLS |
| Scan targets | none | IP literals only |

## Loop guard

With FlClash TUN enabled the app's own TLS would be captured and routed back
into its own SOCKS port. The mihomo profile must contain
`PROCESS-NAME,<app package>,DIRECT`.
