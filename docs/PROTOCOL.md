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

## 5. Android 复验（2026-11-18，P1）

同一份 Go 代码在 Windows（amd64）和 OnePlus Pad 4（android/arm64, API 36）上都跑通了真实隧道：

| 事实 | Windows | Android 16 (OPD2514) |
|---|---|---|
| `api.protonmail.ch` 可达、HTTP/1.1 | ✅ | ✅ 2784 台逻辑服务器 |
| SRP 会话 + token 刷新 | ✅ | ✅ 复用同一会话 + 证书 |
| Stealth TLS（utls Chrome） | ✅ | ✅ SNI `gateway.icloud.com` / `connectivitycheck.gstatic.com` |
| WireGuard 握手 | 1.75 s（首次）/ 250 ms（复用证书） | **251 ms** |
| 隧道内 TCP | ✅ | ✅ `api.ipify.org` → `185.98.169.29` |
| netstack UDP | ❌ | ❌（P2） |

### 5.1 Android 上必须注意的两点

1. **DNS**：Android 没有 `/etc/resolv.conf`，纯 Go 解析器会退回 `[::1]:53` 并失败。
   APK 里 gomobile 产出的库是 cgo 构建（走 bionic 的 `getaddrinfo`），所以不受影响；
   非 cgo 的调试二进制必须用 `-dns <server>` / `SetAPIDNS()` 固定解析器。
2. **SNI**：与 Windows 相同，从 China-safe 池按入口 IP 轮转；真机上命中的是
   `gateway.icloud.com` 与 `connectivitycheck.gstatic.com`，都不是国内 SNI 黑名单里的名字。
