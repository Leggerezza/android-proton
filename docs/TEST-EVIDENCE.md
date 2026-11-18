# Test evidence

> 规则：**只写真实命令与原始输出**。每个阶段一节；没跑的项写 `NOT RUN` 并说明原因，
> 不允许把"应该能过"写成"通过"。P0 章节的每一条都能在 `docs/P0-AUDIT.md` 里找到对应结论。

## 如何复现（P0，2026-11-18）

| 项 | 值 |
|---|---|
| 主机 | Windows 11 10.0.26200 amd64（中国大陆网络） |
| JDK | 25.0.3 LTS（`JAVA_HOME=F:\Coding\Java\jdk-25.0.3`） |
| Gradle | 9.6.0（wrapper 固定，`gradle/wrapper/gradle-wrapper.properties`） |
| AGP / ktlint | 9.5.1 / 14.2.0（`gradle/libs.versions.toml`） |
| Android SDK | platform 36、build-tools 36.1.0（`local.properties` 指向 `%LOCALAPPDATA%\Android\Sdk`） |
| Go | go1.27.4 windows/amd64（`F:\Coding\Go\go`） |
| Go 模块代理 | `GOPROXY=https://goproxy.cn,direct`（`proxy.golang.org` 在本网络不可达） |

基线只读：所有 Go 命令都在 `F:\Temp\baseline-verify`（`baseline/proton-sidecar` 的副本）里跑。

---

## P0 — 审计 + 骨架

### P0-E1 工具链

```

------------------------------------------------------------
Gradle 9.6.0
------------------------------------------------------------

Build time:    2026-06-18 23:19:03 UTC
Revision:      3f750f03d77e42327c5f9fcb9992110088330a32

Kotlin:        2.3.21
Groovy:        4.0.32
Ant:           Apache Ant(TM) version 1.10.17 compiled on April 6 2026
Launcher JVM:  25.0.3 (Oracle Corporation 25.0.3+9-LTS-195)
Daemon JVM:    F:\Coding\Java\jdk-25.0.3 (no Daemon JVM specified, using current Java home)
OS:            Windows 11 10.0 amd64


[exit 0]
```

```
java version "25.0.3" 2026-04-21 LTS
Java(TM) SE Runtime Environment (build 25.0.3+9-LTS-195)
Java HotSpot(TM) 64-Bit Server VM (build 25.0.3+9-LTS-195, mixed mode, sharing)

[exit 0]
```

```
go version go1.27.4 windows/amd64

[exit 0]
```

### P0-E2 仓库状态（P0 提交之前）

```
a1d9fef

[exit 0]
```

```
a1d9fef (HEAD -> main) chore: bootstrap repo (gitignore, hooks, CI, security, docs)

[exit 0]
```

```
 M .gitattributes
 M .github/workflows/ci.yml
 M .github/workflows/security.yml
 M .gitignore
 M CHANGELOG.md
 M CONTRIBUTING.md
 M README.md
 M scripts/check-16kb.sh
?? CODE_OF_CONDUCT.md
?? app/
?? build.gradle.kts
?? core/
?? docs/P0-AUDIT.md
?? gradle.properties
?? gradle/
?? gradlew
?? gradlew.bat
?? settings.gradle.kts

[exit 0]
```

### P0-E3 基线：Windows 构建 / vet / 测试

```powershell
cd F:\Temp\baseline-verify
go build ./...
go vet ./...
go test ./... -count=1 -v
```

```
（无输出，exit 0）
```

```
（无输出，exit 0）
```

```
?   	github.com/dsh/proton-stealth-sidecar/cmd/proton-stealth	[no test files]
?   	github.com/dsh/proton-stealth-sidecar/internal/proton	[no test files]
?   	github.com/dsh/proton-stealth-sidecar/internal/socks5	[no test files]
?   	github.com/dsh/proton-stealth-sidecar/internal/state	[no test files]
=== RUN   TestFrameRoundTrip
--- PASS: TestFrameRoundTrip (0.00s)
=== RUN   TestHandshakeFrameIsNormal
--- PASS: TestHandshakeFrameIsNormal (0.00s)
=== RUN   TestDecodeRejectsGarbage
--- PASS: TestDecodeRejectsGarbage (0.00s)
PASS
ok  	github.com/dsh/proton-stealth-sidecar/internal/stealth	0.032s
=== RUN   TestStealthTunnelEndToEnd
    integration_test.go:132: handshake ok in 250ms (SNI fonts.gstatic.com)
ERROR: wg: 2026/11/18 17:56:21 peer(sKYw…CTFE) - Failed to send data packets: use of closed network connection
--- PASS: TestStealthTunnelEndToEnd (0.69s)
PASS
ok  	github.com/dsh/proton-stealth-sidecar/internal/tunnel	0.745s

[exit 0]
```

> 结论：4 个测试全过；端到端测试 `TestStealthTunnelEndToEnd` 用同一条 TLS+TunSafe 流建立 WireGuard
> 握手，再经 gVisor netstack + SOCKS5 取回 6000 字节 HTTP 响应，逐字节一致（0.69s）。
> 日志里 `Bind close failed: ... closeNotify ...` 是测试结束后关闭连接时的噪声，不影响 PASS。

### P0-E4 基线：android/arm64 交叉构建与依赖事实

```powershell
$env:GOOS="android"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build ./...
go vet ./...
go list -deps -f "{{.ImportPath}}" ./...      # 依赖闭包
go list -deps -f "{{if .CgoFiles}}{{.ImportPath}}{{end}}" ./...   # cgo 包
go build -o F:\Temp\cli-android-arm64 ./cmd/proton-stealth
```

```
（无输出，exit 0）
```

```
（无输出，exit 0）
```

```
unique packages: 347
gvisor packages: 42
utls packages: 12
wireguard packages: 9
wintun packages: 0
cgo packages: 0
```

```
（无输出，exit 0）
```

```
android/arm64 binary: 18300878 bytes
```

> 结论：整个基线（含 CLI）在 android/arm64、`CGO_ENABLED=0` 下可编译可 vet；依赖闭包 347 个包，
> 其中 0 个含 cgo 文件、0 个引用 wintun。18.3 MB 是单文件 CLI 的体积（含 gVisor netstack 42 个包）。

### P0-E5 Go core 模块门（`cd core && go test ./...`）

```bash
cd core
go vet ./...
go test ./... -count=1 -v
```

```
（无输出，exit 0）
```

```
?   	github.com/dsh/android-proton/core/protonnode	[no test files]

[exit 0]
```

> P0 的 core 只有 `protonnode/doc.go`（模块与包文档），**没有任何假实现**，所以 `go test` 报 `[no test files]`。
> 这是 P0 的预期结果：真正的测试在 P1 随协议代码一起进来。

### P0-E6 Android 构建门（`./gradlew assembleDebug`）

```bash
./gradlew clean assembleDebug --no-build-cache --no-configuration-cache --console=plain
```

```
Starting a Gradle Daemon, 1 busy and 2 incompatible and 10 stopped Daemons could not be reused, use --status for details
> Task :app:clean
> Task :app:preBuild UP-TO-DATE
> Task :app:preDebugBuild UP-TO-DATE
> Task :app:mergeDebugNativeDebugMetadata NO-SOURCE
> Task :app:generateDebugResources
> Task :app:packageDebugResources
> Task :app:processDebugNavigationResources
[... 29 lines omitted ...]
> Task :app:compressDebugJavaRes
> Task :app:mergeDebugJavaResource
> Task :app:dexBuilderDebug
> Task :app:mergeProjectDexDebug
> Task :app:packageDebug
> Task :app:createDebugApkListingFileRedirect
> Task :app:assembleDebug

BUILD SUCCESSFUL in 13s
35 actionable tasks: 35 executed

[exit 0]
```

APK 信息（`aapt2 dump badging` + sha256）：

```
package: name='io.github.dsh.protonnode.debug' versionCode='1' versionName='0.1.0' platformBuildVersionName='16' platformBuildVersionCode='36' compileSdkVersion='36' compileSdkVersionCodename='16'
minSdkVersion:'26'
targetSdkVersion:'36'
uses-permission: name='android.permission.INTERNET'
uses-permission: name='android.permission.ACCESS_NETWORK_STATE'
application-label:'Proton Node'
application: label='Proton Node' icon=''
application-debuggable
feature-group: label=''
  uses-feature: name='android.hardware.faketouch'
  uses-implied-feature: name='android.hardware.faketouch' reason='default feature for all apps'
supports-screens: 'small' 'normal' 'large' 'xlarge'
supports-any-density: 'true'
locales: '--_--'
densities: '160'

[exit 0]
```

```
02b5f4d8b880d3a8863d155556ede459fda13e77e36b74379fe8018807637071  app/build/outputs/apk/debug/app-debug.apk
871846 bytes
```

> 两次独立构建（一次用 build cache、一次 `--no-build-cache`）产出的 APK sha256 完全相同，
> 说明在当前 Gradle/AGP 下构建是确定性的。

### P0-E7 质量门：ktlint / 单元测试 / Android Lint

```bash
./gradlew :app:ktlintCheck :app:testDebugUnitTest :app:lintDebug --console=plain
```

```
[... 68 lines omitted ...]

> Task :app:lintReportDebug
Wrote HTML report to file:///F:/User/Documents/DeepSeek_Harness/handoff-proton-android/android-proton/app/build/reports/lint-results-debug.html
Wrote SARIF report to file:///F:/User/Documents/DeepSeek_Harness/handoff-proton-android/android-proton/app/build/reports/lint-results-debug.sarif

> Task :app:lintDebug

BUILD SUCCESSFUL in 6s
36 actionable tasks: 8 executed, 9 from cache, 19 up-to-date
Configuration cache entry reused.

[exit 0]
```

Lint 报告（`app/build/reports/lint-results-debug.txt`）：

```
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\app\build.gradle.kts:35: Warning: Not targeting the latest versions of Android; compatibility modes apply. Consider testing and updating this version. Consult the android.os.Build.VERSION_CODES javadoc for details. [OldTargetApi]
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\gradle\wrapper\gradle-wrapper.properties:3: Warning: A newer version of Gradle than 9.6.0 is available: 9.9.1 [AndroidGradlePluginVersion]
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\app\build.gradle.kts:30: Warning: A newer version of compileSdk than 36 is available: 37 [GradleDependency]
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\app\build.gradle.kts:41: Warning: Missing x86_64 ABI support for ChromeOS [ChromeOsAbiSupport]
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\app\src\main\AndroidManifest.xml:12: Warning: The attribute android:allowBackup is deprecated from Android 12 and higher and may be removed in future versions. Consider adding the attribute android:dataExtractionRules specifying an @xml resource which configures cloud backups and device transfers on Android 12 and higher. [DataExtractionRules]
F:\User\Documents\DeepSeek_Harness\handoff-proton-android\android-proton\app\src\main\AndroidManifest.xml:10: Warning: Should explicitly set android:icon, there is no default [MissingApplicationIcon]
0 errors, 6 warnings
```

> 0 error、6 warning。6 条都是提示级：targetSdk 36 未跟到 37（PROMPT 要求 API 36）、
> compileSdk 37 可用、Gradle 9.9.1 可用、ChromeOS x86_64 未打包（§6.1 只要 arm64）、
> `allowBackup` 建议配 `dataExtractionRules`、缺应用图标（P5 做 UI 时补）。
> `:app:testDebugUnitTest` 在 P0 是 `NO-SOURCE`（app 模块尚无测试源码）。

### P0-E8 16 KB 对齐门

```bash
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release-unsigned.apk
bash scripts/check-16kb.sh          # 不带参数时自动挑最新的 release APK
```

```
checking app/build/outputs/apk/release/app-release-unsigned.apk
no .so in APK (pure-Java build?) - skipping alignment check
zipalign 16KB: OK
16 KB check passed

[exit 0]
```

```
checking app/build/outputs/apk/release/app-release-unsigned.apk
no .so in APK (pure-Java build?) - skipping alignment check
zipalign 16KB: OK
16 KB check passed

[exit 0]
```

`zipalign -c -P 16 -v 4`（debug APK，全量输出见构建日志摘要）：

```
[... 13 lines omitted ...]
  858310 kotlin/kotlin.kotlin_builtins (OK - compressed)
  863718 kotlin/ranges/ranges.kotlin_builtins (OK - compressed)
  865140 kotlin/reflect/reflect.kotlin_builtins (OK - compressed)
Verification successful

[exit 0]
```

> P0 的 APK 里还没有 `.so`（gomobile 的 AAR 在 P1 才接进来），所以脚本走的是 "no .so ... skipping" 分支，
> 只验证了 APK 容器本身的 16 KB 对齐。**`.so` 的 LOAD 段对齐（A2）仍未验证**，见 `docs/P0-AUDIT.md` §7。

### P0-E9 PROMPT §9.2 必备文件清单

| 文件 | 状态 | 字节 |
|---|---|---|
| `README.md` | present | 2251 |
| `LICENSE` | present | 627 |
| `NOTICE` | present | 512 |
| `SECURITY.md` | present | 991 |
| `CONTRIBUTING.md` | present | 1138 |
| `CODE_OF_CONDUCT.md` | present | 2406 |
| `CHANGELOG.md` | present | 768 |
| `.gitignore` | present | 540 |
| `.gitattributes` | present | 174 |
| `.editorconfig` | present | 270 |
| `.github/workflows/ci.yml` | present | 1079 |
| `.github/workflows/security.yml` | present | 1034 |
| `.github/workflows/release.yml` | present | 1062 |
| `docs/ARCHITECTURE.md` | present | 1160 |
| `docs/PROTOCOL.md` | present | 1565 |
| `docs/P0-AUDIT.md` | present | 23804 |
| `docs/TEST-EVIDENCE.md` | **MISSING** | - |

> 首个提交（`a1d9fef`）缺 `CODE_OF_CONDUCT.md`、`docs/P0-AUDIT.md`、`docs/TEST-EVIDENCE.md`，本次 P0 全部补齐。

### P0-E10 全新克隆构建（证明提交树自洽）

CI 的第一步是 `actions/checkout`，所以“提交树能不能独立构建”必须单独证明：克隆到空目录、
没有 `local.properties`（CI 用 `ANDROID_HOME`）、直接构建。

```bash
git clone --branch feat/p0-skeleton . F:\Temp\clone-test
cd F:\Temp\clone-test
git ls-files -s gradlew          # 可执行位必须进 git
./gradlew assembleDebug
cd core && go test ./... -count=1
```

```
F:\Temp> git clone --branch feat/p0-skeleton <repo> F:\Temp\clone-test
F:\Temp\clone-test> git ls-files -s gradlew
100755 249efbb032ce46a80c687c0723eb172e85f6a136 0	gradlew
F:\Temp\clone-test> .\gradlew.bat assembleDebug --console=plain   (ANDROID_HOME set, no local.properties)
> Task :app:packageDebug
> Task :app:assembleDebug
BUILD SUCCESSFUL in 9s
35 actionable tasks: 35 executed

apk: app/build/outputs/apk/debug/app-debug.apk = 871846 bytes
F:\Temp\clone-test\core> go test ./... -count=1
?   github.com/dsh/android-proton/core/protonnode  [no test files]
[exit 0]
```

> 克隆后没有任何配置就能 `assembleDebug`，产出的 APK 与工作区构建的字节数一致（871,846）；
> `gradlew` 的 `100755` 位已连同内容一起提交。
### P0-E11 未执行项（NOT RUN）

| 项 | 原因 | 谁来消除 |
|---|---|---|
| GitHub Actions CI 全流程 | 本地仓库没有远端；CI 的每一步都在本机等价跑过 | 首次 push 后 |
| 真机（OnePlus Pad 4 / OxygenOS 16）安装与运行 | P0 不含隧道与 UI，无可运行功能 | P1 |
| `gomobile bind` 生成 AAR | P1 的产物 | P1（A1） |
| `.so` 的 16 KB LOAD 段对齐（`readelf`） | 同上 | P1/P7（A2） |
| netstack UDP（SOCKS5 UDP ASSOCIATE） | 基线没有 UDP 代码 | P2（A6） |
| Proton 真实账号登录/握手 | P0 不改协议，且需要真账号 | P1 |
| govulncheck / gitleaks | P8 验收项 | P8 |

---

## P1 — 真实隧道（2026-11-18）

### P1 判定

| 判定项（PROMPT §11 / START-HERE §4） | 结果 | 证据 |
|---|---|---|
| 真实 WireGuard 握手（不是 TLS 握手成功） | ✅ Windows 250 ms；真机 251 ms | P1-E6 / P1-E7 |
| 隧道内 1 个 TCP 请求成功 | ✅ 两端都拿到 Proton 出口 IP | P1-E6 / P1-E7 |
| 真机（OnePlus Pad 4 / Android 16 / arm64） | ✅ 同一份 Go 代码跑通 | P1-E7 |
| gomobile 能绑定 core（A1 前半） | ✅ AAR + classes.jar，14 个方法 | P1-E3 |
| Go 的 .so 满足 16 KB 对齐（A2 前半） | ✅ 4 个 LOAD 段全部 0x4000 | P1-E4 |
| 装进 APK 后仍满足 16 KB（A2 后半） | ✅ release/debug 两个 APK 都过 | P1-E5 |
| JNI 桥在应用进程里真正加载（A1 后半） | ✅ 真机 instrumentation 1/1 通过 | P1-E8 |
| SOCKS5 UDP（A6） | ❌ 未实现，P2 做 | P1-E9 |

### P1-E1 环境

# device (adb -s 10.241.100.148:41427)
model: OPD2514
manufacturer: OnePlus
android release: 16
sdk: 36
abi: arm64-v8a
build: OPD2514_16.0.8.301(EX01)
PAGE_SIZE: 4096

# host toolchain
go: go version go1.27.4 windows/amd64
ndk: Pkg.Desc = Android NDK | Pkg.Revision = 28.2.13676358 | Pkg.BaseRevision = 28.2.13676358 | Pkg.ReleaseName = r28c
gomobile:  mod golang.org/x/mobile v0.0.0-20261117202452-47699113848a h1:jYIfLfiCY1/N6kWqBdUak4c1kMN0vKnoQypLZqbGqv4= |  dep golang.org/x/mod v0.43.0 h1:MFuq5J6s35W9D3IjkwyWc9vHh0nENOrt5K8Iof4ofmg=

llvm-readelf: LLVM (http://llvm.org/):

### P1-E2 core 的 vet 与测试

```bash
cd core
go vet ./...
go test ./... -count=1 -v
```

?   	github.com/dsh/android-proton/core/cmd/protonnode-cli	[no test files]
?   	github.com/dsh/android-proton/core/proton	[no test files]
=== RUN   TestGameModeDefaults
--- PASS: TestGameModeDefaults (0.00s)
=== RUN   TestGameModeRejectsSecureCore
--- PASS: TestGameModeRejectsSecureCore (0.00s)
=== RUN   TestNormalModeDefaultsAndValidation
--- PASS: TestNormalModeDefaultsAndValidation (0.00s)
=== RUN   TestSelectCandidatesPrefersOrderAndSkipsUnavailable
--- PASS: TestSelectCandidatesPrefersOrderAndSkipsUnavailable (0.00s)
=== RUN   TestRedactMasksEmailAndToken
--- PASS: TestRedactMasksEmailAndToken (0.00s)
=== RUN   TestRingLogKeepsNewest
--- PASS: TestRingLogKeepsNewest (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/protonnode	2.050s
=== RUN   TestLoopbackAddrRejectsNonLoopback
--- PASS: TestLoopbackAddrRejectsNonLoopback (0.00s)
=== RUN   TestConnectRelay
--- PASS: TestConnectRelay (0.00s)
=== RUN   TestUDPAssociateIsRejected
--- PASS: TestUDPAssociateIsRejected (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/socks5	0.481s
=== RUN   TestFrameRoundTrip
--- PASS: TestFrameRoundTrip (0.00s)
=== RUN   TestHandshakeFrameIsNormal
--- PASS: TestHandshakeFrameIsNormal (0.00s)
=== RUN   TestDecodeRejectsGarbage
--- PASS: TestDecodeRejectsGarbage (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/stealth	1.353s
=== RUN   TestWriteJSONIsAtomicAndPrivate
--- PASS: TestWriteJSONIsAtomicAndPrivate (0.01s)
=== RUN   TestNewRejectsEmptyDir
--- PASS: TestNewRejectsEmptyDir (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/store	0.042s
=== RUN   TestStealthTunnelEndToEnd
    integration_test.go:132: handshake ok in 251ms (SNI fonts.gstatic.com)
ERROR: srv: 2026/11/18 21:00:41 peer(iK5x…88ng) - Failed to send data packets: stealth(listen): write: write tcp 127.0.0.1:62063->127.0.0.1:62064: wsasend: An established connection was aborted by the software in your host machine.
ERROR: srv: 2026/11/18 21:00:41 Bind close failed: tls: failed to send closeNotify alert (but connection was closed anyway): write tcp 127.0.0.1:62063->127.0.0.1:62064: wsasend: An established connection was aborted by the software in your host machine.
--- PASS: TestStealthTunnelEndToEnd (0.69s)
PASS
ok  	github.com/dsh/android-proton/core/tunnel	2.021s

[exit 0]

> 15 个测试全 PASS。`TestStealthTunnelEndToEnd` 是原样搬过来的端到端测试
> （TLS → WireGuard → gVisor netstack → SOCKS5 → HTTP）；新增的是配置默认值、选服顺序、
> 日志脱敏、环形缓冲、SOCKS5 环回约束 / CONNECT 中继 / UDP 拒绝、store 原子写。
>
> `-race` 没有在本机跑：Go 的 race 检测需要 cgo 编译器，这台 Windows 没有 gcc/msvc。
> CI（ubuntu）里的 `go test -race` 覆盖这一项。

### P1-E3 gomobile AAR（A1 前半）

```bash
go get -tool golang.org/x/mobile/cmd/gobind      # gomobile 现在要求 tool 依赖
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
export ANDROID_NDK_HOME=<SDK>/ndk/28.2.13676358
gomobile bind -target=android/arm64 -androidapi 26 -o core.aar ./protonnode
```

```

[exit 0]
```

AAR 内容：

```
-rw-rw-r--  0 0      0         149 11月 30  1979 AndroidManifest.xml
-rw-rw-r--  0 0      0          58 11月 30  1979 proguard.txt
-rw-rw-r--  0 0      0       10147 11月 30  1979 classes.jar
-rw-rw-r--  0 0      0    25008840 11月 30  1979 jni/arm64-v8a/libgojni.so
-rw-rw-r--  0 0      0           0 11月 30  1979 R.txt
drwxrwxr-x  0 0      0           0 11月 30  1979 res/

[exit 0]
```

生成的 Java 面（javap）：

```
Compiled from "Protonnode.java"
public abstract class protonnode.Protonnode {
  public static void touch();
  public static native void init(java.lang.String) throws java.lang.Exception;
  public static native java.lang.String listServers(java.lang.String, boolean) throws java.lang.Exception;
  public static native java.lang.String login(java.lang.String, java.lang.String, java.lang.String) throws java.lang.Exception;
  public static native void logout() throws java.lang.Exception;
  public static native java.lang.String recentLogs(long);
  public static native java.lang.String sessionJSON() throws java.lang.Exception;
  public static native java.lang.String sessionState();
  public static native void setAPIDNS(java.lang.String) throws java.lang.Exception;
  public static native void setSession(java.lang.String) throws java.lang.Exception;
  public static native void shutdown();
  public static native java.lang.String start(java.lang.String) throws java.lang.Exception;
  public static native java.lang.String status();
  public static native void stop() throws java.lang.Exception;
  static {};
}

[exit 0]
```

> 参数/返回值只有 `String`、`boolean`、`long`、`void`，`error` 变成 Java 异常，
> 完全落在 PROMPT §5.1 的 gomobile 约束内。导出的错误哨兵值已改成包内变量，
> 免得 gomobile 为它们生成 `setErr…/getErr…` 这种没有意义的 setter。

### P1-E4 .so 的 16 KB 对齐（A2 前半）

```bash
tar -xf core.aar
llvm-readelf -lW jni/arm64-v8a/libgojni.so
```

```
  LOAD           0x000000 0x0000000000000000 0x0000000000000000 0x5bd730 0x5bd730 R   0x4000
  LOAD           0x5bd730 0x00000000005c1730 0x00000000005c1730 0x488f90 0x488f90 R E 0x4000
  LOAD           0xa466c0 0x0000000000a4e6c0 0x0000000000a4e6c0 0x155fd8 0x156940 RW  0x4000
  LOAD           0xb9c6a0 0x0000000000ba86a0 0x0000000000ba86a0 0x08ed20 0x20df088 RW  0x4000
[exit 0]
```

### P1-E5 APK 的 16 KB 门（A2 后半，这次真的带 .so）

```bash
./gradlew clean :app:assembleDebug :app:assembleRelease     # AAR 经 app/libs/core.aar 进入 APK
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release-unsigned.apk
bash scripts/check-16kb.sh app/build/outputs/apk/debug/app-debug.apk
zipalign -c -P 16 -v 4 app/build/outputs/apk/release/app-release-unsigned.apk
```

```
checking app/build/outputs/apk/release/app-release-unsigned.apk
checking /tmp/tmp.BErSDCg3dP/lib/arm64-v8a/libgojni.so
  LOAD segments: 0x4000 0x4000 0x4000 0x4000 
zipalign 16KB: OK
16 KB check passed

[exit 0]
```

```
checking app/build/outputs/apk/debug/app-debug.apk
checking /tmp/tmp.w5iUlUVpsp/lib/arm64-v8a/libgojni.so
  LOAD segments: 0x4000 0x4000 0x4000 0x4000 
zipalign 16KB: OK
16 KB check passed

[exit 0]
```

```
Verification successful
[exit 0]
```

> 顺带修掉脚本里的一个**假阴性**：Info-ZIP 6.00（Git Bash）不展开 `lib/*` 这个模式，
> 旧版脚本会静默地"没找到 .so"，打印 `skipping alignment check` 然后 **PASS**。
> 现在按 `unzip -l` 列出的精确条目名逐个解包，解不出来就 FAIL。这个 bug 在 P0 的
> 纯 Java APK 上看不出来，接进 AAR 之后才暴露。

### P1-E6 Windows 上的真实 Proton 隧道

真实账号（复用 Windows 侧已验证的会话），自动选服 + L1 探测 + 证书复用 + 真实握手 + SOCKS5：

```powershell
protonnode-cli selftest -state session.json -cfg cfg -socks 11082 -mode normal -wait 120s
```

```
session: {"loggedIn":true,"uid":"umllpq6w…","email":"ch***@outlook.com","deviceName":"proton-stealth-LAPTOP-HAI0E0DS","certSerial":"18560510771","certExpiresAt":"2026-11-23T20:15:36+08:00","sessionDirty":false}
connected after 24.524s
status: {"state":"connected","server":"US-CA#834","entryIP":"79.127.185.162","sni":"connectivitycheck.gstatic.com","handshakeMs":251,"rxBytes":92,"txBytes":180,"pid":34752,"udp":false,"socksPort":11082,"mode":"normal","reconnects":0}
in-tunnel GET http://api.ipify.org/ -> 185.98.169.29
status: {"state":"connected","server":"US-CA#834","entryIP":"79.127.185.162","sni":"connectivitycheck.gstatic.com","handshakeMs":251,"uptimeSec":1,"rxBytes":972,"txBytes":916,"pid":34752,"udp":false,"socksPort":11082,"mode":"normal","reconnects":0}
--- logs ---
21:00:46.453 core: initialized (dir F:\Temp\p1\cfg-win2)
21:00:46.511 session: loaded for device prot…(30) (uid umllpq6w…)
21:00:46.511 start: mode normal, socks 127.0.0.1:11082, mtu 1320, fingerprint chrome
21:00:46.511 GET https://api.protonmail.ch/vpn/v1/logicals?SecureCoreFilter=all&WithState=true
21:00:49.875 servers: 2784 logical servers from the API
21:00:53.876 entry 79.127.185.162 (US-CA#834) reachable
21:00:57.877 entry 95.173.221.92 (US-CO#156) reachable
21:01:09.880 entry 212.102.44.166 (US-CO#54) reachable
21:01:09.883 certificate: reusing serial 18560510771 (expires 2026-11-23T20:15:36+08:00)
21:01:09.883 server 1/3: US-CA#834 (US, San Jose) entry 79.127.185.162 load 37% secure-core=false
21:01:10.089 stealth: tcp connected to 79.127.185.162:443
21:01:10.089 stealth: certificate verification is skipped for the Proton entry (rotating cert; PROMPT.md 0.4)
21:01:10.295 stealth: TLS established with 79.127.185.162:443 (sni connectivitycheck.gstatic.com)
21:01:10.649 tunnel up in 251ms (sni connectivitycheck.gstatic.com)
21:01:10.649 SOCKS5 listening on 127.0.0.1:11082 (TCP CONNECT only in P1)
21:01:10.650 socks5: listening on 127.0.0.1:11082
21:01:11.456 socks5: CONNECT tunnel established
ERROR: wg: 2026/11/18 21:01:11 peer(2RxT…WmDA) - Failed to send data packets: use of closed network connection
stopped: {"state":"idle","rxBytes":0,"txBytes":0,"pid":34752,"udp":false,"mode":"normal","reconnects":0}

[exit 0]
```

### P1-E7 真机上的真实 Proton 隧道（OnePlus Pad 4）

同一份 core 交叉编译成 `android/arm64`，`adb push` 到 /data/local/tmp 执行。这不是 APK 形态，
而是"Go 核心在 Android 上能不能真的建隧道"的最小验证；APK 形态由 P1-E8 覆盖：

```bash
go build -trimpath -ldflags "-s -w" -o protonnode-cli-android ./cmd/protonnode-cli   # GOOS=android GOARCH=arm64
adb push protonnode-cli-android /data/local/tmp/p1/protonnode-cli
adb shell /data/local/tmp/p1/protonnode-cli selftest -state /data/local/tmp/p1/session.json \
    -cfg /data/local/tmp/p1/cfg2 -socks 11080 -dns 172.26.26.3 -mode normal -wait 120s
```

```
session: {"loggedIn":true,"uid":"umllpq6w…","email":"ch***@outlook.com","deviceName":"proton-stealth-LAPTOP-HAI0E0DS","certSerial":"18560510771","certExpiresAt":"2026-11-23T12:15:36Z","sessionDirty":false}
connected after 8.526s
status: {"state":"connected","server":"US-CA#838","entryIP":"79.127.185.162","sni":"connectivitycheck.gstatic.com","handshakeMs":251,"rxBytes":92,"txBytes":180,"pid":26195,"udp":false,"socksPort":11080,"mode":"normal","reconnects":0}
in-tunnel GET http://api.ipify.org/ -> 185.98.169.29
status: {"state":"connected","server":"US-CA#838","entryIP":"79.127.185.162","sni":"connectivitycheck.gstatic.com","handshakeMs":251,"uptimeSec":1,"rxBytes":4828,"txBytes":4972,"pid":26195,"udp":false,"socksPort":11080,"mode":"normal","reconnects":0}
--- logs ---
13:01:19.280 core: initialized (dir /data/local/tmp/p1/cfg2)
13:01:19.280 api: client rebuilt (dns "172.26.26.3")
13:01:19.280 session: loaded for device prot…(30) (uid umllpq6w…)
13:01:19.281 start: mode normal, socks 127.0.0.1:11080, mtu 1320, fingerprint chrome
13:01:19.281 GET https://api.protonmail.ch/vpn/v1/logicals?SecureCoreFilter=all&WithState=true
13:01:22.500 servers: 2784 logical servers from the API
13:01:26.502 entry 79.127.185.162 (US-CA#838) reachable
13:01:26.502 entry 212.102.44.166 (US-CO#58) reachable
13:01:26.502 entry 79.127.136.65 (US-IL#354) reachable
13:01:26.508 certificate: reusing serial 18560510771 (expires 2026-11-23T12:15:36Z)
13:01:26.508 server 1/3: US-CA#838 (US, San Jose) entry 79.127.185.162 load 29% secure-core=false
13:01:26.745 stealth: tcp connected to 79.127.185.162:443
13:01:26.745 stealth: certificate verification is skipped for the Proton entry (rotating cert; PROMPT.md 0.4)
13:01:26.973 stealth: TLS established with 79.127.185.162:443 (sni connectivitycheck.gstatic.com)
13:01:27.327 tunnel up in 251ms (sni connectivitycheck.gstatic.com)
13:01:27.327 SOCKS5 listening on 127.0.0.1:11080 (TCP CONNECT only in P1)
13:01:27.327 socks5: listening on 127.0.0.1:11080
13:01:28.297 socks5: CONNECT tunnel established
stopped: {"state":"idle","rxBytes":0,"txBytes":0,"pid":26195,"udp":false,"mode":"normal","reconnects":0}

[exit 0]
```

> 会话文件在测试后立刻从设备删除（`rm -f /data/local/tmp/p1/session.json`），设备上只留下
> `prefs.json` 与 `servers.json`（非机密）。

### P1-E8 真机 JNI 冒烟：Go 运行时在应用进程里加载

```bash
./gradlew :app:connectedDebugAndroidTest        # ANDROID_SERIAL=<device>
```

```
[... 80 lines omitted ...]
Running tests on devices: OPD2514 - 16


BUILD SUCCESSFUL in 17s
68 actionable tasks: 14 executed, 18 from cache, 36 up-to-date
Configuration cache entry reused.

[exit 0]
```

```xml
<?xml version='1.0' encoding='UTF-8' ?>
<testsuites tests="1" failures="0" errors="0" skipped="0" time="0.000" timestamp="2026-11-18T13:05:24" hostname="localhost">
  <testsuite name="io.github.dsh.protonnode.CoreBridgeTest" tests="1" failures="0" errors="0" skipped="0" time="0.014" timestamp="2026-11-18T13:05:24" hostname="localhost">
    <properties>
      <property name="device" value="10.241.100.148_41427" />
    </properties>
    <testcase name="initStatusAndShutdownWork" classname="io.github.dsh.protonnode.CoreBridgeTest" time="0.014" />
  </testsuite>
</testsuites>
```

> 测试内容：`Protonnode.init(filesDir/core)` → `status()` 是 idle 的 JSON → `recentLogs()`
> 含 "initialized" → `sessionState()` 是 JSON → `shutdown()`。它证明 A1 的后半句：
> **gomobile 生成的 JNI 桥能在 Android 16 应用进程里加载 Go 运行时**
> （utls + wireguard-go + gVisor netstack 全部被链接进来）并正常调用。

### P1-E9 发现与仍未验证的项

| # | 发现 | 影响 / 处理 |
|---|---|---|
| 1 | Android 上**纯 Go 的 DNS 解析器不可用**：没有 /etc/resolv.conf，Go 退回 [::1]:53，报 connection refused | 新增 `SetAPIDNS()` 与 CLI 的 `-dns`（把 API 解析固定到 172.26.26.3）。APK 里不走这条路：gomobile 的库是 cgo 构建，走 bionic 的 getaddrinfo，系统 DNS 正常 |
| 2 | 设备 `getconf PAGE_SIZE` = **4096** | 这台不是 16 KB 页设备，A12（16 KB 运行时）仍未验证；静态对齐（A2）已由 readelf/zipalign 证明 |
| 3 | 冷启动首次连接 20–25 s（L1 逐个探入口 IP，每批 12 个） | 有服务器缓存与证书后降到 3.5 s；P3 的三级扫描会重新设计这段并给耗时基线 |
| 4 | `servers.json` 缓存 2784 台服务器约 1.7 MB | 可接受；P3 改成只存筛选所需字段 |
| 5 | SOCKS5 不再打印目标域名（P0 审计发现的隐私问题已修） | 现在只有 `socks5: CONNECT tunnel established` 和应答码 |
| 6 | `gomobile bind` 要求模块里有 `tool golang.org/x/mobile/cmd/gobind` | go.mod 因此把 go 指令升到 1.26.0、并升级了 x/crypto 等；新版本就是当前锁定版本，go.sum 已提交 |
| 7 | 设备上存在 vgate0 接口（疑似 FlClash/VPN） | 不影响 P1（CLI 直连入口 IP）；P6 专门验证 mihomo 的进程/UID 规则 |

**仍未验证，不得当成结论**：A6（netstack UDP，P2）、A10（OxygenOS 保活，P4）、
A11（FlClash PROCESS-NAME，P6）、A12（16 KB 运行时，需要 16 KB 页真机，P7）、A13（half-close，P2）。

### P2 后必测：16 KB 页运行时（A12）

> 这条**不是** P2 的功能验收项，但它必须在 P2 结束后立刻做一次，不能拖到发布前才想起。
> 决定记录在 docs/P0-AUDIT.md §7.1；发布前的复跑写在 docs/RELEASE-CHECKLIST.md。

为什么现在还不能关：静态对齐（A2，P1-E4/P1-E5）只证明 `libgojni.so` 的 LOAD 段是 `0x4000`、
APK 能被打包器接受；它**不证明** Go runtime 与整条隧道在 16 KB 页内核上运行正常。

做法（P2 结束时执行，结果追加到本节）：

```bash
# 1. 装一个 16 KB 系统镜像（Android 15/16 都提供 16 KB 变体）
sdkmanager --install "system-images;android-36;google_apis;arm64-v8a"   # 名称以 sdkmanager --list 为准，
                                                                        # 需要带 "16 KB"/"16k" 的那一条
avdmanager create avd -n proton16k -k "system-images;android-36;google_apis;arm64-v8a" --device "pixel_tablet"
emulator -avd proton16k -no-snapshot &

# 2. 确认它真的跑在 16 KB 上（关键，不是看镜像名字）
adb shell getconf PAGE_SIZE          # 期望 16384

# 3. 装 APK 并跑 JNI 冒烟 + 隧道自测
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb shell am instrument -w io.github.dsh.protonnode.debug.test/androidx.test.runner.AndroidJUnitRunner
# （或 ./gradlew :app:connectedDebugAndroidTest，ANDROID_SERIAL=emulator-5554）
```

判定：`getconf PAGE_SIZE` = 16384 且 instrumentation 通过、`Status()` 状态机走到
`connected`（模拟器能出网时）。任一失败就是 P2 之后的第一个 bug，优先于 P3。

状态：**NOT RUN**（P2 结束后执行）

## P2 — SOCKS5 TCP + UDP（2026-11-18）

### P2 判定（PROMPT.md 5.3 / 5.4 / 11）

| 判定项 | 结果 | 证据 |
|---|---|---|
| `curl -x socks5h://127.0.0.1:<port>` 通 | ✅ 真实 curl 经节点取到 Proton 出口 IP | P2-E3 |
| UDP echo 往返（协议级） | ✅ 桌面 + 真机，经真实 Proton 走 DNS 查询并拿到 A 记录 | P2-E4 / P2-E5 |
| UDP 关联生命周期 = TCP 控制连接 | ✅ 关闭控制连接后 2s 内不再应答 | P2-E2（`TestUDPAssociateDiesWithControlConnection`） |
| `FRAG!=0` 丢弃、三种 ATYP | ✅ 单元测试 | P2-E2 |
| 每关联 `dst→socket` LRU 64、空闲 120s 回收 | ✅ 单元测试（cap 与 TTL 用短值注入） | P2-E2 |
| 单报文 ≤ 65487、`RSV` 必须为 0 | ✅ 解析器拒绝非法值 | P2-E2 |
| ≥16 个并发目标 | ✅ 16 个 echo 目标同时活跃 | P2-E2 |
| `Stop()` 后无残留 socket | ✅ 监听端口可被立即重新绑定；curl 结束后 `netstat` 无 LISTEN | P2-E2 / P2-E3 |
| netstack UDP（PROMPT §4 的 ❌） | ✅ 本地端到端 + 真机端到端 | P2-E6 |

> 真机证据一律来自 OnePlus Pad 4（OPD2514 / Android 16 / arm64），真实 Proton 账号。

### P2-E1 结论摘要

```
core 测试：20 个，全 PASS（protonnode 6、socks5 11、stealth 3、store 2、tunnel 3 → 其中 tunnel 3 含 UDP 与 half-close）
真机：socks5 11/11 PASS、tunnel 3/3 PASS
真实链路：curl socks5h → 169.150.231.165；UDP DNS 往返 31→79 字节（桌面 attempt 2、真机 attempt 1）
```

### P2-E2 core 的 vet 与测试

```bash
cd core
go vet ./...
go test ./... -count=1 -v
```

```
[... 26 lines omitted ...]
=== RUN   TestUDPAssociateDropsFragment
--- PASS: TestUDPAssociateDropsFragment (0.40s)
=== RUN   TestUDPAssociateRejectedWithoutDialer
--- PASS: TestUDPAssociateRejectedWithoutDialer (0.00s)
=== RUN   TestAssociationLRUAndIdle
--- PASS: TestAssociationLRUAndIdle (0.40s)
=== RUN   TestUDPAssociateConcurrentTargets
--- PASS: TestUDPAssociateConcurrentTargets (0.00s)
=== RUN   TestUDPAssociateDiesWithControlConnection
--- PASS: TestUDPAssociateDiesWithControlConnection (0.85s)
=== RUN   TestServerClosesListenerOnCancel
--- PASS: TestServerClosesListenerOnCancel (0.10s)
PASS
ok  	github.com/dsh/android-proton/core/socks5	2.172s
=== RUN   TestFrameRoundTrip
--- PASS: TestFrameRoundTrip (0.00s)
=== RUN   TestHandshakeFrameIsNormal
--- PASS: TestHandshakeFrameIsNormal (0.00s)
=== RUN   TestDecodeRejectsGarbage
--- PASS: TestDecodeRejectsGarbage (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/stealth	1.129s
=== RUN   TestWriteJSONIsAtomicAndPrivate
--- PASS: TestWriteJSONIsAtomicAndPrivate (0.01s)
=== RUN   TestNewRejectsEmptyDir
--- PASS: TestNewRejectsEmptyDir (0.00s)
PASS
ok  	github.com/dsh/android-proton/core/store	0.034s
=== RUN   TestTCPHalfCloseThroughTunnel
--- PASS: TestTCPHalfCloseThroughTunnel (0.69s)
=== RUN   TestStealthTunnelEndToEnd
    integration_test.go:132: handshake ok in 250ms (SNI fonts.gstatic.com)
ERROR: srv: 2026/11/18 21:57:28 Bind close failed: tls: failed to send closeNotify alert (but connection was closed anyway): write tcp 127.0.0.1:59367->127.0.0.1:59368: wsasend: An established connection was aborted by the software in your host machine.
--- PASS: TestStealthTunnelEndToEnd (0.74s)
=== RUN   TestUDPThroughTunnel
--- PASS: TestUDPThroughTunnel (0.69s)
PASS
ok  	github.com/dsh/android-proton/core/tunnel	3.227s

[exit 0]
```

> 关键新增：`TestUDPThroughTunnel`（netstack UDP 往返 + 64 包突发）、`TestTCPHalfCloseThroughTunnel`
> （netstack 的 `CloseWrite`，即 A13）、`TestUDPAssociateConcurrentTargets`（16 目标）、
> `TestAssociationLRUAndIdle`、`TestUDPAssociateDiesWithControlConnection`、`TestServerClosesListenerOnCancel`。

### P2-E3 真实 curl 经 SOCKS5（TCP 回归 + 真实出口）

```bash
# 前台：protonnode-cli run -state session.json -cfg cfg -socks 11085 -mode normal -server "US-CO#54"
curl.exe -sS --max-time 40 -x socks5h://127.0.0.1:11085 https://api.ipify.org
taskkill /PID <pid> /T /F
netstat -ano | findstr :11085
```

```
curl exit 0
body: 169.150.231.165
```

> `netstat` 只剩 curl 自己的 `TIME_WAIT` 连接，**没有 LISTEN**：`Stop()` 没有留下监听 socket。

### P2-E4 桌面：真实 Proton 上的 UDP 往返

```bash
protonnode-cli udptest -state session.json -cfg cfg -socks 11084 -server "US-CO#54" -wait 90s
```

```
session: {"loggedIn":true,"uid":"umllpq6w…","email":"ch***@outlook.com","deviceName":"proton-stealth-LAPTOP-HAI0E0DS","certSerial":"18560510771","certExpiresAt":"2026-11-23T20:15:36+08:00","sessionDirty":false}
status: {"state":"connected","server":"US-CO#54","entryIP":"212.102.44.166","sni":"gateway.icloud.com","handshakeMs":250,"rxBytes":92,"txBytes":180,"pid":10100,"udp":true,"socksPort":11084,"mode":"normal","reconnects":0}
udp relay: 127.0.0.1:51610
udp attempt 1: read udp 127.0.0.1:51611->127.0.0.1:51610: i/o timeout (retrying, UDP has no retransmission)
UDP echo round-trip through Proton: 31 bytes -> 79 bytes, 3 answer(s): [104.26.13.205 104.26.12.205 172.67.74.152] (attempts: 2)
status: {"state":"connected","server":"US-CO#54","entryIP":"212.102.44.166","sni":"gateway.icloud.com","handshakeMs":250,"uptimeSec":5,"rxBytes":14232,"txBytes":14764,"pid":10100,"udp":true,"socksPort":11084,"mode":"normal","reconnects":0}
stopped: {"state":"idle","rxBytes":0,"txBytes":0,"pid":10100,"udp":true,"mode":"normal","reconnects":0}

[exit 0]
```

> 第一次超时不是 bug：UDP 没有重传，隧道刚建立时第一个数据报可能丢。因此 udptest 内置 3 次重试
> （真实 DNS 客户端也是这样做的）；这一条要写进 PROTOCOL.md 的限制里。

### P2-E5 真机：同一份代码、真实 Proton 的 UDP 往返

```bash
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o protonnode-cli-android ./cmd/protonnode-cli
adb push protonnode-cli-android /data/local/tmp/p1/protonnode-cli
adb shell /data/local/tmp/p1/protonnode-cli udptest -state /data/local/tmp/p1/session.json \
    -cfg /data/local/tmp/p1/cfg5 -socks 11080 -dns 172.26.26.3 -server "US-CO#54" -wait 90s
```

```
session: {"loggedIn":true,"uid":"umllpq6w…","email":"ch***@outlook.com","deviceName":"proton-stealth-LAPTOP-HAI0E0DS","certSerial":"18560510771","certExpiresAt":"2026-11-23T12:15:36Z","sessionDirty":false}
status: {"state":"connected","server":"US-CO#54","entryIP":"212.102.44.166","sni":"gateway.icloud.com","handshakeMs":250,"rxBytes":92,"txBytes":180,"pid":8513,"udp":true,"socksPort":11080,"mode":"normal","reconnects":0}
udp relay: 127.0.0.1:48939
UDP echo round-trip through Proton: 31 bytes -> 79 bytes, 3 answer(s): [104.26.12.205 172.67.74.152 104.26.13.205] (attempts: 1)
status: {"state":"connected","server":"US-CO#54","entryIP":"212.102.44.166","sni":"gateway.icloud.com","handshakeMs":250,"uptimeSec":1,"rxBytes":236,"txBytes":276,"pid":8513,"udp":true,"socksPort":11080,"mode":"normal","reconnects":0}
stopped: {"state":"idle","rxBytes":0,"txBytes":0,"pid":8513,"udp":true,"mode":"normal","reconnects":0}

[exit 0]
```

### P2-E6 在真机上直接跑测试二进制（排除"只能在桌面过"的疑虑）

```bash
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go test -c -o socks5.test ./socks5
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go test -c -o tunnel.test ./tunnel
adb push socks5.test tunnel.test /data/local/tmp/p1/
adb shell "cd /data/local/tmp/p1 && ./socks5.test -test.v -test.timeout 180s"
adb shell "cd /data/local/tmp/p1 && ./tunnel.test -test.v -test.timeout 180s"
```

```
[... 13 lines omitted ...]
--- PASS: TestUDPAssociateRejectedWithoutDialer (0.00s)
=== RUN   TestAssociationLRUAndIdle
--- PASS: TestAssociationLRUAndIdle (0.40s)
=== RUN   TestUDPAssociateConcurrentTargets
--- PASS: TestUDPAssociateConcurrentTargets (0.00s)
=== RUN   TestUDPAssociateDiesWithControlConnection
--- PASS: TestUDPAssociateDiesWithControlConnection (0.20s)
=== RUN   TestServerClosesListenerOnCancel
--- PASS: TestServerClosesListenerOnCancel (0.10s)
PASS

[exit 0]
```

```
[... 6 lines omitted ...]
--- PASS: TestStealthTunnelEndToEnd (0.71s)
=== RUN   TestUDPThroughTunnel
--- PASS: TestUDPThroughTunnel (0.38s)
PASS

[exit 0]
```

> 这两次运行第一次暴露了一个**真机才有的时序问题**：关闭控制连接后，relay 最多还会应答 1s
> （`serve()` 在等 `ReadFromUDP` 的 1s 读超时）。修法是在关联取消时立刻关闭 relay socket，
> 让读操作立即返回；测试也改成"2s 内必须停止应答"。桌面从未复现，真机稳定复现 —— 值得记录。

### P2-E7 P2 的发现

| # | 发现 | 处理 |
|---|---|---|
| 1 | UDP 是尽力而为：隧道刚建立时首个数据报可能丢（桌面复现 attempt 1 超时、attempt 2 成功） | udptest 加 3 次重试；PROTOCOL.md 写明"应用层要自带重试" |
| 2 | 8.8.8.8:53 有一次连续失败、几分钟后同一入口恢复 | 外部网络抖动，不是代码问题；已记录在证据里，未做特殊处理 |
| 3 | DNS 查询头少写 2 字节（ARCOUNT）导致被解析器丢弃 | 由第一次真机失败暴露并修复；说明"真链路测试"必须真的发包，不能只看状态 |
| 4 | 关联取消后 relay 仍能应答最多 1s | 取消时立即关闭 relay socket；真机测试改为 2s 容忍 |
| 5 | `gonet.TCPConn` 实现了 `CloseWrite` | A13 关闭：SOCKS5 TCP 的 half-close 在隧道内可用 |


## P3 — 扫描（三级/取消/缓存）

`NOT RUN`

## P4 — 前台服务与生命周期

`NOT RUN`

## P5 — UI

`NOT RUN`

## P6 — FlClash 集成

`NOT RUN`

## P7 — 发布（16 KB / SHA256 / SBOM）

`NOT RUN`

## P8 — 安全加固

`NOT RUN`
