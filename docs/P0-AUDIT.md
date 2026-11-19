# P0 依赖审计 — baseline/proton-sidecar → Android 16 (API 36, arm64-v8a)

> 阶段：**P0**（只做审计 + 仓库骨架；不含任何 Android 隧道/UI 代码）
> 基线：`baseline/proton-sidecar`（Windows 上真实 Proton 跑通过的 Go 实现，见其 `EVIDENCE-real-proton.md`）
> 审计日期：2026-11-18  ·  基线快照：`android-proton` @ `a1d9fef`（骨架提交之前）
> 证据：所有 [实测] 项的可复现命令与原始输出都在 [TEST-EVIDENCE.md](TEST-EVIDENCE.md#p0)。

结论的三种强度，本文每条都标注：

| 标记 | 含义 |
|---|---|
| **[实测]** | 本次跑过命令，输出留档 |
| **[源码]** | 读基线源码得出的事实（附文件:行号） |
| **[推断]** | 合理推断，未验证 → 必须进 §7 假设清单，不得当成结论 |

---

## 0. 结论摘要

START-HERE 说“四个 Go 包”，实际基线是 **5 个库包 + 1 个 CLI**（`proton` `stealth` `tunnel` `socks5` `state` `cmd/proton-stealth`），本审计全部覆盖。

| 包 | 行数 | 判定 | 一句话 | 阶段 |
|---|---:|---|---|---|
| `internal/stealth` | 1043 | **可直接复用** | TunSafe 帧 / 密钥派生 / SNI 池 / TLS-Bind 全部纯 Go | P1 |
| `internal/tunnel` | 467 | **可直接复用**（运行期待复验） | gVisor netstack，无 TUN、无 root，与 Android 形态天然一致 | P1 |
| `internal/proton` | 1057 | **需改写** | API/SRP/证书逻辑可复用；DPAPI 导入与 Windows 路径必须删除 | P1 |
| `internal/socks5` | 281 | **需扩展** | 只有 TCP CONNECT；UDP ASSOCIATE / 512 上限 / half-close 全缺 | P2 |
| `internal/state` | 189 | **需重写** | 明文 JSON 存 token，Android 上必须换成 Keystore 加密 | P1(最小)/P4 |
| `cmd/proton-stealth` | 1428 | **Android 不可用** | CLI/控制台/tasklist/终端密码；策略逻辑需抽取 | P1/P3/P4 |

**最重要的三个结论**

1. **编译器层面已经可移植**：整个基线（含 CLI）在 `GOOS=android GOARCH=arm64 CGO_ENABLED=0` 下 `go build ./...` 与 `go vet ./...` 全绿，依赖闭包 **0 个含 cgo 文件的包**，且**没有引入 wintun**。[实测]
   → “能不能编到 Android”已经不是风险；剩下的风险全在**运行时行为**与**契约层**（§7）。
2. **协议不用重写**：帧格式、密钥派生、SNI 策略、netstack TCP 在 Windows 上已用真实隧道验证，审计未发现其中隐藏 Windows 假设。P1 的任务是**搬运 + 在 Android 上重新取一次证据**，不是重新实现。
3. **差距集中在“契约面”**：PROMPT §5.1 的 17 个 gomobile 函数在基线里**一个都没有**（基线是 CLI），等价物散落在 `cmd/proton-stealth/main.go` 的选服、探测、健康检查、重连里。P1 第一件事是建 `core/protonnode` 契约层，而不是抄协议代码。

---

## 1. 审计方法（可复现）

基线是只读参考，**不在原地跑**（避免污染 `baseline/`）。做法是复制到临时目录再验证：

```powershell
xcopy baseline\proton-sidecar F:\Temp\baseline-verify\ /E /I /Q /Y
cd F:\Temp\baseline-verify
go build ./...                 # windows/amd64
go vet ./...
go test ./... -count=1 -v
$env:GOOS="android"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build ./...                 # android/arm64
go vet ./...
```

工具链（本机实测，原始输出见 `docs/TEST-EVIDENCE.md` P0-E1）：

| 组件 | 版本 | 备注 |
|---|---|---|
| Go | go1.27.4 windows/amd64 | 与基线 `go.mod` 的 `go 1.27.4` 一致 |
| JDK | 25.0.3 LTS | 构建与测试都用它（CI 同步为 25） |
| Gradle | 9.6.0（wrapper 固定） | 本机缓存已有该发行版 |
| AGP | 9.5.1 | 当前稳定版；AGP 9 内置 Kotlin，无需 `kotlin-android` 插件 |
| Android SDK | platform 36 / build-tools 36.1.0 | `compileSdk=36` |
| ktlint | 14.2.0（Gradle 插件） | pre-commit 与 CI 的 lint 面 |
| 网络 | 中国大陆联通/移动；Go 模块走 `goproxy.cn` | proxy.golang.org 不可达 [实测] |

> 说明：基线 `go.sum` 固定的版本全部能拉取并编译通过 [实测]；P1 移植必须沿用同一批版本号，不允许 `go get -u`（PROMPT §9.3 依赖固定）。

---

## 2. 平台事实（实测，先于一切设计）

| 项 | 结果 | 证据 |
|---|---|---|
| 基线 Windows 构建 | `go build ./...` / `go vet ./...` exit 0 | P0-E3 |
| 基线 Windows 测试 | 4 个测试全过；端到端 TLS→WireGuard→netstack→SOCKS5→HTTP 6000 字节逐字节一致，0.69s | P0-E3 |
| 基线 Android/arm64 交叉构建 | `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...` / `go vet ./...` exit 0 | P0-E4 |
| cgo 依赖 | **0 个**含 cgo 文件的包（`go list -deps` 全量扫描） | P0-E4 |
| 依赖闭包 | 347 个包（gvisor 42、utls 12、wireguard 9） | P0-E4 |
| wintun | **未进入依赖图** | P0-E4 |
| android/arm64 单文件体积 | 18,300,878 字节（含 CLI、utls、gvisor netstack） | P0-E4 |
| 仓库骨架构建 | `./gradlew clean assembleDebug --no-build-cache`：35 个 task 全部执行，BUILD SUCCESSFUL，APK 871,846 字节，sha256 `02b5f4d8…7071` | P0-E6 |
| 16 KB 对齐 | `zipalign -c -P 16 -v 4` 通过；APK 内暂无 `.so`（“no .so in APK - skipping alignment check”） | P0-E6 |

> **注意**：“0 个 cgo 包”是**必要条件不是充分条件**：gomobile 的 `-target=android/arm64` 还要过 c-shared 链接、符号导出、`.so` 的 16 KB LOAD 段对齐三道关（§7 A1/A2）。

---

## 3. 逐包审计

### 3.1 `internal/proton`（1057 行）— **需改写**

| 文件 | 行数 | 判定 | 说明与 Android 动作 |
|---|---:|---|---|
| `client.go` | 356 | **复用**（改代理语义） | `Transport()`（client.go:122，NextProtos 在 :135）已强制 `NextProtos=["http/1.1"]`、`DialContext` 支持固定 IP。`proxy env/URL` 是桌面概念 → Android 删除，改为“直连 + 可选 DoH 固定 IP”（§5.6 角色 3）。`do()` 的重试 / 401 刷新单飞（client.go:207、307）原样用。 |
| `auth.go` | 153 | **复用** | `Login`（auth.go:56）走 SRP，密码不出进程；`Submit2FA`（auth.go:119）。契约要求一次调用 `Login(user,pass,totp)` 30s 可取消 → 需合并（P1）。`needs2FA()`（auth.go:99）的 scope 判定需真账号复验（§7 A8）。 |
| `cert.go` | 110 | **复用**（改设备名） | `RequestCertificate`（cert.go:69）Ed25519 PKIX + 7 天；`Fresh()`（cert.go:50）提前 24h 续期。`DefaultDeviceName()`（cert.go:59）用 `os.Hostname()` → Android 改 `Build.MODEL` + 持久化随机后缀。 |
| `logicals.go` | 267 | **复用** | 纯数据+排序，无平台 API。v1 带 Score/Load 是选服依据。 |
| `settings.go` + `dpapi_windows.go` + `dpapi_other.go` | 98+50+9 | **Android 不可用** | DPAPI（crypt32）与 Proton 桌面 `GlobalSettings.json` 只在 Windows 存在。Android 唯一入口是 **SRP 登录 + TOTP**；三个文件整体删除。 |
| `b64.go` | 7 | 复用 | 无平台依赖 |

**结论**：协议与 API 逻辑一行都不用改；要改的是“会话怎么来”（DPAPI → SRP）与“走不走代理”。

### 3.2 `internal/stealth`（1043 行）— **可直接复用**

| 文件 | 行数 | 判定 | 说明 |
|---|---:|---|---|
| `framing.go` | 172 | **复用（逐字节）** | TunSafe 2 字节头 + 数据帧压缩（`encode`:framing.go:65 / `decode`:114）。纯 `encoding/binary` + `io`。 |
| `framing_test.go` | 89 | **复用** | 3 个测试：整包往返、握手帧不压缩、坏帧报错。P1 直接搬进 `core/stealth`，再补 §10.3 的固定向量。 |
| `keys.go` | 72 | **复用** | Ed25519 seed → `SHA512(seed)[:32]` clamp → X25519（keys.go:54），与 Proton `go-vpn-lib` 一致。 |
| `names.go` | 170 | **复用** | 全量 SNI 池 + China-safe 子集 + 按 IP 轮转（names.go:160）。国内用全量池会 RST（代码注释来自实测）。 |
| `bind.go` | 327 | **复用（3 处要改）** | 实现 `wireguard/conn.Bind`。① `InsecureSkipVerify: true`（bind.go:260）是 Proton 入口的必要行为，PROMPT §0.4 允许但要求**记日志一次** → P1 加“首次跳过校验”审计事件；② `SetMark`（bind.go:211）在 Android 是空操作 → 保留但注释；③ `receive()`（bind.go:157）把**所有**读错误包成 `temporary`（bind.go:327），包括 `io.EOF` → P1 要区分 `EOF/closed` 与可重试错误，否则收包循环可能空转。 |
| `listenbind.go` | 207 | **复用（仅测试）** | 服务端对照实现，只被 `tunnel/integration_test.go` 使用。生产代码不得引用。 |
| `time.Sleep(100ms)`（bind.go:281） | — | 保留 + 注释 | Proton 客户端握手后固定睡眠；PROMPT §5.2 禁的是“裸 sleep 循环”，这是单次协议延时 → 写成具名常量并记入 PROTOCOL.md。 |

### 3.3 `internal/tunnel`（467 行）— **可直接复用**

| 文件 | 行数 | 判定 | 说明 |
|---|---:|---|---|
| `tunnel.go` | 211 | **复用** | `tnetstack.CreateNetTUN`（tunnel.go:88）建用户态网络栈：**不建 TUN、不要 root、不改路由**。`DialContext`（192）只允许 `tcp`，域名交给隧道内 DNS 10.2.0.1（不泄漏）。`Stats()`（164）读 UAPI 计数器 → 直接喂 `Status()`。 |
| `integration_test.go` | 256 | **复用** | 本地起 TLS+WireGuard 服务端把 client 全栈跑通，是 P1 的确定性验收基线（不需要 Proton 账号）。 |

**缺口（P2 从零做）**：UDP。netstack 具备 UDP 能力，但基线**一次都没用**；PROMPT §4 也把 netstack UDP 记为 ❌（§7 A6）。

### 3.4 `internal/socks5`（281 行）— **需扩展**

| 事实 | 位置 | 对 P2 的含义 |
|---|---|---|
| 只实现 `CMD=0x01` | socks5.go:98 `handle` | `0x02→0x07` 已符合 PROMPT；`0x03`（UDP ASSOCIATE）**整段缺失** |
| UDP 直接拒绝 | socks5.go:114 | 新增 `udp.go`：关联生命周期、LRU 64、空闲 120s、`FRAG!=0` 丢弃、三种 ATYP |
| 无连接上限 | socks5.go:57 `ListenAndServe` | PROMPT 要 512 上限 |
| `relay()` 的 half-close | socks5.go:265 | 用 `CloseWrite()` 断言；netstack 的 `net.Conn` 是否实现未验证（§7 A13） |
| 监听地址来自参数 | socks5.go:57 | 契约要求**只允许 127.0.0.1**，P2 里硬校验 |

### 3.5 `internal/state`（189 行）— **需重写**

| 事实 | 位置 | 判定 |
|---|---|---|
| token 明文 JSON 落盘 | state.go:114 `Save()`、state.go:131 `os.WriteFile(tmp, buf, 0o600)` | **违反 PROMPT §0.5** → Android 不可接受 |
| 路径来自 Windows 环境 | state.go:41 `CandidatePaths`（LOCALAPPDATA/UserConfigDir/Executable/Temp） | Android 无此语义；改为 `Init(cfgDir)` 注入 |
| Ed25519 seed 与 token 同文件 | state.go:161 `KeyPair()` | 一起加密；seed 丢失 = 重新注册设备（占设备位） |
| 服务器列表缓存 7 天 | state.go:143 `ServerCache` | 与扫描缓存（30min）是**两个东西**，P3 不要混用 |

**设计决定（P1 落地）**：加密放 Kotlin（Keystore 密钥不可导出），Go 的 `store` 只负责“原子写一个不透明 blob”，加解密由 App 层在调用 `Init/SetSession` 时完成。§0.5 由 P8 的 Keystore 验证收口。

### 3.6 `cmd/proton-stealth`（1428 行）— **Android 不可用，但逻辑要抽出来**

不可用部分：`flag` CLI、`os/signal`（main.go:533）、`term.ReadPassword`、`exec.Command("tasklist", ...)`（main.go:1040）、`hideConsole()`（hide_windows.go）、`syscall.SIGTERM`。

**必须抽进 core 的策略逻辑**（P1/P3/P4 的真实资产，别重写）：

| 逻辑 | 位置 | 去向 |
|---|---|---|
| 单实例 / 端口占用判定 | main.go:773 `serveTunnel` | 服务层（P4） |
| 30s 探针 ×2 失败才判定断线 | main.go:804 `monitorTunnel` | `core/protonnode` 健康检查（§2 断线策略） |
| 重连循环（60s）+ 重新选服 | main.go:686–724 | 状态机 `reconnecting`（§5.7） |
| API 失败无限退避重试 | main.go:849 `withRetry` | 登录/证书阶段（P1） |
| 入口 IP 去重 + 12 并发探测 | main.go:1228 `reachableCandidates` | 扫描 L1（P3），现在是 ad-hoc |
| 选服候选拼装（国家偏好 → 全表兜底） | main.go:581–605 | §7 选服策略（P3） |
| 隧道内自测（HTTP GET 取出口 IP） | main.go:1291 `fetchThroughTunnel` | P1 验收 / `Status()` 的 exitIp |
| mihomo 配置生成 | main.go:1339 `cmdMihomo` | 不需要（配置在 repo 的 `mihomo/`） |

**游戏模式的差异必须重写**（不是移植）：基线是“2 次失败 → 60s 后重连并**重新选服**”，PROMPT §7 要求游戏中**先重试同一入口**、连续 3 轮（≈3 分钟）失败才换服并明确提示。基线里没有这个概念。

---

## 4. 第三方依赖审计

| 模块 | 版本 | 用途 | Android 判定 | 依据 |
|---|---|---|---|---|
| `refraction-networking/utls` | v1.8.2 | Chrome ClientHello 指纹 | **复用** | 纯 Go；android/arm64 构建通过 [实测]；运行期待 P1 |
| `zx2c4.com/wireguard` | v0.0.0-20261024220952-b3bc704ef0ee | `device` + `tun/netstack` | **复用** | 只用 `tun/netstack`，不碰 `tun/wintun`；依赖图无 wintun [实测] |
| `gvisor.dev/gvisor` | v0.0.0-20250503011706-39ed1f5ac29c | 用户态 TCP/IP 栈 | **复用**（体积/性能待测） | 42 个包参与；未在 Android 实测 |
| `ProtonMail/go-srp` | v0.0.7 | SRP 登录 | **复用** | 间接依赖 bcrypt/circl/saferith 全纯 Go |
| `golang.org/x/crypto` | v0.37.0 | curve25519 / 派生 | 复用 | 纯 Go |
| `golang.org/x/term` | v0.48.0 | CLI 密码输入 | **Android 不可用** | 只被 `cmd/` 使用 → 不进 core |
| `zx2c4.com/wintun` | — | Windows TUN 驱动 | **禁止引入** | 未进入依赖图 [实测]；core 禁止 import `wireguard/tun` |
| `x/net` / `x/sys` / `x/time` | v0.39.0 / v0.50.0 / v0.7.0 | 间接 | 复用 | — |

**govulncheck 尚未跑**（P8 验收项）；P0 只做“依赖固定”这一半。

---

## 5. 平台 API 依赖清单（Android 上必须替换）

| 位置 | 用法 | Android 替代 |
|---|---|---|
| `proton/dpapi_windows.go`:27、`proton/settings.go`:20 | crypt32 DPAPI + Proton 桌面配置 | **删除**；改 SRP 登录（TOTP） |
| `state/state.go`:41 | LOCALAPPDATA / UserConfigDir / Executable / Temp | `Init(cfgDir)` 注入 `context.filesDir` |
| `proton/cert.go`:59 | `os.Hostname()` 作设备名 | `Build.MODEL` + 随机后缀（持久化） |
| `cmd/main.go`:1040 | `tasklist` 查进程 | 不需要 |
| `cmd/main.go`:533 | `signal.NotifyContext` | `Service.onDestroy → Stop()`（§5.2） |
| `cmd/main.go`:367 | `term.IsTerminal/ReadPassword` | 系统 UI 输入框（不落盘） |
| `cmd/hide_windows.go`:1 | kernel32/user32 | 无 |
| `stealth/bind.go`:211 | `SetMark`（SO_MARK） | 空实现（无 VpnService） |
| `state/state.go`:131 | `0o600` 文件权限 | 沙箱目录已私有；敏感数据仍走 Keystore |
| `stealth/bind.go`:281 | 握手后 sleep 100ms | 保留（协议行为），写成具名常量并记入 PROTOCOL.md |

---

## 6. 与 PROMPT §5 契约的差距（P1 的施工图）

| §5.1 契约 | 基线现状 | 差距 |
|---|---|---|
| `Init(cfgDir)` / `Shutdown()` | 无 | 新建（注入目录、初始化 store/日志环） |
| `Login(user, pass, totp)` | `Login` + `Submit2FA` 两个调用 | 合并 + 30s context + Cancel |
| `Logout()` | 有 | 直接复用 |
| `SessionState()` | 无（CLI `status` 打印） | 新建（JSON） |
| `ListServers(country, secureCore)` | `cmdServers` 内联 | 抽成返回 JSON |
| `StartScan/ScanProgress/ScanResults/CancelScan/LastScanSummary` | 只有 `cmdProbe`（L1 TCP + ad-hoc TLS） | **P3 新建**：三级、ID、进度、取消 ≤1s、30min 缓存 |
| `Start(cfgJSON)` / `Stop()` / `Status()` | `cmdRun` 的阻塞循环 | 非阻塞 Start + 原子快照 Status + 幂等 Stop ≤5s |
| `RecentLogs(n)` | `log.Printf` 直出 stderr | 新建 500 行环形缓冲 + 脱敏 |
| `ExportConfig/ImportConfig` | 无 | P5 新建（schema 校验 + 版本） |

**状态机**（§5.7 `idle→scanning→connecting→connected→reconnecting→idle`）：基线只有隐式的
“connect → serve → monitor → reconnect”，没有对外可观测状态。P1 必须先把状态机显式化，否则 P4/P5 无法验收。

---

## 7. 未验证假设（编号，逐条给验证方法）

| # | 假设 | 为什么现在是假设 | 验证方法 | 阶段 |
|---|---|---|---|---|
| A1 | `gomobile bind -target=android/arm64` 能绑定 wireguard-go + gvisor | 只验证了 `go build`（无 cgo），没验证 c-shared 链接与导出 | `gomobile bind -target=android/arm64 -o core.aar ./protonnode` 后 `unzip -l` 确认 `lib/arm64-v8a/libgojni.so` | P1 |
| A2 | Go 产出的 `.so` 每个 LOAD 段 `Align 0x4000` | 16 KB 页设备要求；Go 版本行为未测 | `readelf -lW libgojni.so` + `scripts/check-16kb.sh` 跑真 APK | P1/P7 |
| A3 | gVisor netstack 在 arm64 手机的吞吐/CPU 可接受 | 只在 Windows/amd64 跑过 | 隧道内 iperf3/下载测速 + CPU 观察 | P1 |
| A4 | utls Chrome 指纹 + China-safe SNI 在国内网络能完成 TLS 握手 | Windows 通过，Android 网络/MTU 不同 | 真机 TLS handshake OK 计数 > 0 | P1 |
| A5 | 隧道内 DNS 10.2.0.1 能解析域名 | netstack 侧未在 Android 跑过 | 隧道内解析域名并取回 200 | P1 |
| A6 | netstack UDP 可用（SOCKS5 UDP ASSOCIATE 前提） | 基线完全没有 UDP 代码 | 本地 UDP echo + 真实游戏 UDP 联机 | P2 |
| A7 | `api.protonmail.ch` 在 Android 上 HTTP/1.1-only 行为一致 | Windows 实测；Android 环境不同 | P1 登录 + `/vpn/v2` 200 | P1 |
| A8 | `needs2FA()` 的 scope 判定对真实账号成立 | 只有源码推断 | 用开 2FA 的账号登录 | P1 |
| A9 | 同一 `DeviceName` 重复注册不会快速耗尽设备位 | 基线每次换 state 会新建设备 | 记录设备列表变化 | P1 |
| A10 | OxygenOS 16 的前台服务能长期存活 | 未测 | 锁屏 2h / 游戏 30min 观察 | P4 |
| A11 | FlClash `PROCESS-NAME,<包名>` 能命中本应用与 GameNative | 未测 | mihomo 连接面板看 UID/进程 | P6 |
| A12 | 16 KB 页运行时 | ✅ **已验证（2026-11-19）**：16 KB 页的 Android 16 guest（`PAGE_SIZE=16384`）上，JNI 冒烟 OK(1 test)、socks5 11/11、tunnel 3/3；arm64 真机待 P7 复跑（模拟器的 arm64 翻译 SIGILL） | TEST-EVIDENCE「P2 后必测」 |
| A13 | netstack 的 `net.Conn` 支持 `CloseWrite`（half-close） | 未验证 | 单元测试：半关闭后对端能读完 | P2 |

---

### 7.1 P1 复验结果（2026-11-18，真实账号 + 真机）

| # | 假设 | 状态 | 证据 |
|---|---|---|---|
| A1 | gomobile 能绑定 core | ✅ **已验证**：bind 出 AAR，真机 instrumentation 里 JNI 加载并调用成功 | TEST-EVIDENCE P1-E3 / P1-E8 |
| A2 | Go 的 .so 满足 16 KB 对齐 | ✅ **已验证**：4 个 LOAD 段全 0x4000，且 release/debug APK 的 16 KB 门通过 | P1-E4 / P1-E5 |
| A3 | netstack 吞吐/CPU 可接受 | ⏳ 只证明了功能可用，没有吞吐基线（P1 不做性能验收） | P1-E7 |
| A4 | utls Chrome 指纹 + China-safe SNI 在国内网络可握手 | ✅ **已验证**：真机 TLS + WG 握手 251 ms（SNI gateway.icloud.com / connectivitycheck.gstatic.com） | P1-E7 |
| A5 | 隧道内 DNS 10.2.0.1 能解析 | ✅ 间接验证：隧道内 GET api.ipify.org（域名）返回出口 IP | P1-E7 |
| A6 | netstack UDP 可用 | ✅ **已验证（P2）**：本地端到端 + 真机端到端，真实 Proton 上 DNS 查询走 UDP 拿到 A 记录 | TEST-EVIDENCE P2-E2 / P2-E4 / P2-E5 |
| A7 | api.protonmail.ch 的 HTTP/1.1-only 行为在 Android 一致 | ✅ **已验证**：真机拉到 2784 台逻辑服务器 | P1-E7 |
| A8 | needs2FA 的 scope 判定 | ⏳ 未用开 2FA 的账号验证（P1 复用了已有会话） | — |
| A9 | 同一 DeviceName 不耗尽设备位 | ⏳ 只观察了证书复用，没有核对账号设备列表 | — |
| A10 | OxygenOS 前台服务保活 | ❌ P4 | — |
| A11 | FlClash PROCESS-NAME 命中 | ❌ P6 | — |
| A12 | 16 KB 页运行时 | 🕓 **P2 结束后必测一次**（16 KB 系统镜像模拟器跑 APK + 自测），发布前（P7）再复跑；本机 OPD2514 是 4 KB 内核，测不了 | P1-E9、TEST-EVIDENCE「P2 后必测」 |
| A13 | netstack 的 CloseWrite（half-close） | ✅ **已验证（P2）**：TestTCPHalfCloseThroughTunnel 半关闭后仍能读回响应 | TEST-EVIDENCE P2-E2 |

> 结论：P1 把 A1/A2/A4/A5/A7 从「假设」变成了「证据」，A6/A10/A11/A12/A13 仍必须在各自阶段消掉。
> **A12 的处理（2026-11-18 决定）**：不等 P7 —— **P2 一结束就用 16 KB 系统镜像的模拟器测一次**，
> 把结果写进 TEST-EVIDENCE；发布前（P7）在 `docs/RELEASE-CHECKLIST.md` 里再复跑一次作为发布门禁。
> 本机 OPD2514 的 `getconf PAGE_SIZE` 是 4096（`pagesize.max=16384` 说明硬件支持，但当前是 4 KB 内核），
> 所以不能用它来消 A12。

## 8. 风险登记

| 风险 | 影响 | 缓解 | 阶段 |
|---|---|---|---|
| gVisor 体积（18.3 MB 单文件） | APK 25–30 MB，低端机安装/启动慢 | 只保留 arm64 + R8；P7 记录体积基线 | P7 |
| 单条 TLS 流承载全部流量 | 丢包 → TCP 重传 + HOL 阻塞，游戏延迟抖动 | README 写“适合可达性不适合竞技”；P2 实测 | P2 |
| 游戏中换服导致会话全断 | 玩家掉线 | §7 修正策略：先重试同入口，3 轮才换服 | P4 |
| 后台被杀 | 节点消失，FlClash 上游失效 | 前台服务 + 通知 + 自启动引导 | P4 |
| Proton 改协议 | 隧道突然失效 | 参数集中在 `core/stealth`，PROTOCOL.md 记录 | 持续 |
| 设备注册上限 | 设备位耗尽 | seed/证书加密持久化，不轻易重置 | P1 |
| 明文 token（基线做法） | 违反 §0.5 | Keystore AES-GCM（Kotlin 侧） | P4/P8 |

---

## 9. P1 的移植顺序（文件级映射）

```
baseline/proton-sidecar                       →  android-proton/core
──────────────────────────────────────────────────────────────────────────────
internal/stealth/framing.go        (原样)     →  stealth/framing.go
internal/stealth/framing_test.go   (原样)     →  stealth/framing_test.go
internal/stealth/keys.go           (原样)     →  stealth/keys.go
internal/stealth/names.go          (原样)     →  stealth/names.go
internal/stealth/bind.go           (3 处改)   →  stealth/bind.go
internal/stealth/listenbind.go     (原样)     →  stealth/listenbind.go   (仅测试)
internal/tunnel/tunnel.go          (原样)     →  tunnel/tunnel.go
internal/tunnel/integration_test.go(原样)     →  tunnel/integration_test.go
internal/proton/client.go          (改代理)   →  proton/client.go
internal/proton/auth.go            (合并2FA)  →  proton/auth.go
internal/proton/cert.go            (改设备名) →  proton/cert.go
internal/proton/logicals.go        (原样)     →  proton/logicals.go
internal/proton/b64.go             (原样)     →  proton/b64.go
internal/proton/settings.go ─┐
internal/proton/dpapi_*.go   ─┴──  删除（Android 无 DPAPI）
internal/socks5/socks5.go          (P2 扩展)  →  socks5/tcp.go
internal/state/state.go            (重写)     →  store/store.go         (不透明 blob)
cmd/proton-stealth/main.go         (抽取)     →  protonnode/session.go, protonnode/status.go
                                   (新建)     →  protonnode/api.go       (§5.1 契约)
                                   (新建)     →  scan/*.go              (P3)
```

P1 的验收命令（写进 TEST-EVIDENCE 的 P1 章节）：

```bash
cd core && go vet ./... && go test ./... -count=1 -race
cd .. && ./gradlew :app:assembleDebug          # 带 AAR
adb push                                      # 真机：登录 → 握手 → 隧道内 curl https://api.ipify.org
```

---

## 10. P0 的落地决定与偏差（都要写进评审）

| 决定 | 理由 |
|---|---|
| **AGP 9.5.1 + Gradle 9.6.0**，不用 PROMPT §6.1 提到的 “AGP 8.9+” | AGP 8.9 只支持到 Gradle 8.x，与 JDK 25 组合跑不动；§6.1 的实质要求是 **16 KB 支持**，由 AGP≥8.5.1 + `useLegacyPackaging=false` 满足，已实测 `zipalign -c -P 16` 通过。 |
| `core/go.mod` 写 `go 1.24.0`，CI 用 Go 1.27.4 | §6.1 要求 “Go 1.24+”；基线锁定 1.27.4。go.mod 取最低可用，CI/本机取基线版本，两者都能编。 |
| `applicationId = io.github.dsh.protonnode`，debug 加 `.debug` 后缀 | 包名是 FlClash `PROCESS-NAME` 规则的键（§5.6 防环路），必须先定死；debug 后缀让真机可并存安装（§7 A11）。 |
| ktlint 14.2.0 落地（Gradle 插件） | §9.5 的 pre-commit 与 CI 都要求 ktlint；`./gradlew :app:ktlintCheck` 实测通过。 |
| CI 用 JDK 25 | 与本地证据一致；AGP 9 要求 JDK 17+。 |
| P0 **不写任何 UI、不写隧道代码** | §0.2 阶段门：P1/P2 未过禁止 UI。骨架里只有一个空 `Application` 类用于验证 Kotlin 工具链。 |
| 基线在 `F:\Temp\baseline-verify` 副本上验证 | `baseline/` 是只读参考（START-HERE §5.1）。 |
| 未跑 CI | 本地无 GitHub 远端。CI 要跑的每一步都在本地跑过，命令与输出见 TEST-EVIDENCE。 |

### P0 验收对照

| START-HERE §3 要求 | 状态 | 证据 |
|---|---|---|
| 1) 四个 Go 包依赖审计 | ✅（覆盖 5 个库 + CLI） | 本文 §3、§4 |
| 2) 补 Gradle 骨架 + `core/go.mod`，两命令可跑 | ✅ | TEST-EVIDENCE P0-E5/E6 |
| 3) `docs/P0-AUDIT.md`、`docs/TEST-EVIDENCE.md` P0 章节 | ✅ | 本文件 / 对应文件 |
| PROMPT §9.2 必备文件 | ✅（补了 CODE_OF_CONDUCT） | TEST-EVIDENCE P0-E8 |
| 不实现 UI / 不实现隧道 | ✅ | 无 Activity；core 只有 doc.go |

---

## 11. 下一步（P1 的第一天）

1. 建 `core/protonnode` 契约层（§5.1 函数签名 + JSON 编解码 + 状态机枚举），先只有骨架与状态机测试。
2. 原样搬 `stealth/{framing,keys,names}` + `tunnel`，跑通 `go test ./... -race`（含端到端测试）。
3. 跑 A1/A2：`gomobile bind` + `readelf`，结果写进 TEST-EVIDENCE 的 P1 章节。
4. 改 `bind.SetMark` / 错误分类 / 首次跳过校验日志，再搬 `proton`（删 DPAPI）。
5. 真机：登录 → 选服 → 证书 → 握手 → 隧道内 `curl https://api.ipify.org`，**出口 IP 与本人公网 IP 不同**才算 P1 过。
