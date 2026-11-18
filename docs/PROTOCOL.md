# Proton Stealth protocol notes

Verified against a real Proton account on Windows
(`proton-sidecar/EVIDENCE-real-proton.md`). This is the reference for the Android port.

## 1. Control plane

1. `POST /auth/info` (SRP parameters) -> `POST /auth/v4` (proof) -> tokens.
2. `GET /vpn/v1/logicals?SecureCoreFilter=all&WithState=true` -> logical servers.
3. `POST /vpn/v1/certificate` with an Ed25519 PKIX PEM -> device certificate (7 days).

Host: `https://api.protonmail.ch` (canonical `vpn-api.proton.me` is SNI-blocked in CN).
**ALPN must be http/1.1 only.**

## 2. Data plane

```
TCP connect :443 (entry IP)
TLS 1.3 handshake (utls Chrome fingerprint, SNI from a benign pool)
  -> sleep 100 ms
  -> TunSafe frame stream
       byte0: bits[7:6] type (00 = full WG packet, 10 = data frame)
              bits[5:0] payload length high
       byte1: payload length low
       data frames omit the 16-byte WireGuard data header; the receiver
       rebuilds it from the previous prefix + a 32-bit counter
WireGuard handshake -> keepalive 25s -> MTU 1280..1320
```

## 3. Keys

- Ed25519 seed -> X25519 secret: `SHA512(seed)[:32]` clamped.
- Peer public key for the tunnel = `X25519PublicKey` of the physical server from
  `logicals` (not the certificate's `ServerPublicKey`).

## 4. Failure modes

| Symptom | Meaning |
|---|---|
| TLS handshake EOF immediately | SNI blacklisted or entry IP blocked |
| TCP connect timeout | IP blackholed |
| TLS ok, WG handshake timeout | key rejected or path throttled |
| handshake ok, no data | MTU / routing inside the tunnel |
