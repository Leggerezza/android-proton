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

## P1 — 真实隧道

`NOT RUN`（P0 只做审计与骨架；按 PROMPT §0.2 不得提前开始）

预期要产出的证据：真机 `gomobile bind` 的 AAR 内容、`.so` 的 `readelf` LOAD 对齐、
隧道内 `curl https://api.ipify.org` 的出口 IP、`Status()` JSON。

## P2 — SOCKS5 TCP + UDP

`NOT RUN`

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
