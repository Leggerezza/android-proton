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

## 6. SOCKS5 UDP ASSOCIATE（P2 实现）

```
TCP 控制连接: VER|0x03|RSV|ATYP|DST|PORT      -> VER|0x00|RSV|ATYP|BND|PORT
UDP 报文:     RSV(2)=0|FRAG|ATYP|DST|PORT|DATA
```

| 规则 | 实现 |
|---|---|
| relay 绑定 | `127.0.0.1:0`，把真实 BND 回给客户端 |
| 生命周期 | 控制连接关闭 → 立即关闭 relay socket 与全部上游 socket |
| `FRAG != 0` | 丢弃（不支持分片重组） |
| ATYP | IPv4 / 域名 / IPv6 全支持；域名在**隧道内** 10.2.0.1 解析（5s 预算） |
| 会话表 | 每关联 `dst → UDPConn`，上限 64，LRU 淘汰，空闲 120s 回收 |
| 并发 | 16 个以上目标同时活跃已验证（要求 ≥16） |
| 报文大小 | 单报文 ≤ 65507 字节 |

**UDP 是尽力而为**：隧道刚建立时首个数据报可能丢（桌面实测 3 次里第 1 次超时）。
应用层（DNS 客户端、游戏）必须自带重试——这不是 bug，是 UDP + 新建隧道的正常行为。

## 7. 服务器扫描（P3）

| 级别 | 做什么 | 超时 | 通过意味着 |
|---|---|---|---|
| L1 `tcp` | 对入口 IP 做 TCP 连接 | 3 s | 地址活着（**不代表**能用） |
| L2 `tls` | utls Chrome ClientHello + China-safe SNI 轮转 + 真实 TLS 握手 | 5 s | Stealth 终结者接得住 |
| L3 `tunnel` | 完整 WireGuard 握手（用账号的 X25519 私钥） | 12 s | 对端接受我们的密钥，**只有它能用于自动选服** |

编排：L1 全量（并发 32）→ 按 score/load 取前 24 个做 L2 → 按 RTT 取前 3 个做 L3。
结果按 L3 RTT 排序，写入 ``scan-cache.json``，键 = 网络指纹 + 级别，TTL 30 分钟；网络切换后不复用。
取消：每个 worker 在每一步之间检查 context，``CancelScan`` 实测 1 ms 内生效。

实测（2026-11-19，JP）：13 个入口 → L1 13 → L2 若干 → **L3 2**（212.102.51.121 / .122，握手 1.48 s / 1.52 s）。

## 8. 网络路径与 FlClash（证据前提）

本应用自己**不走**系统代理：Go 的 transport 设 `Proxy=nil`，扫描用裸 `net.Dialer`。
所以 Windows 上 FlClash 的 system-proxy（127.0.0.1:7890）不会影响我们的测量；但 **TUN/VpnService 会**。

- 开发机：FlClash 是 system-proxy 模式，默认路由仍在 WLAN → 我们的直连证据成立（见 TEST-EVIDENCE「网络路径自检」）。
- 手机：FlClash 的 `vgate0` 一旦 up，shell/本应用的连接会被 TUN 接管 → 设备侧证据必须在 VPN 关闭时采集；
  这也是 P6 要验证的 `PROCESS-NAME,<包名>,DIRECT` 规则（否则我们的 API/隧道会被自己的 VPN 再抓一次）。
