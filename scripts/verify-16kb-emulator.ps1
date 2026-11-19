<#
.SYNOPSIS
    Verify the app on a real 16 KB page-size Android environment (A12).

.DESCRIPTION
    Boots the Android 16 "16 KB page size" emulator image, proves the guest really
    runs on 16 KB pages (getconf PAGE_SIZE = 16384), installs the debug APK and the
    instrumentation test, then runs the JNI smoke test.

    Run it from an INTERACTIVE desktop session (normal terminal or Android Studio).
    An emulator started from a service / non-interactive session can stay offline
    forever.

    The script never throws on an "adb ... offline" message: while the guest is
    booting adb answers with errors, which is expected and is polled in a loop.

.PARAMETER Port
    Emulator console port (adb serial becomes emulator-<Port>). Default 5556.

.PARAMETER BootTimeoutSec
    How long to wait for sys.boot_completed. Default 600.

.PARAMETER GpuMode
    Passed to -gpu. Default swiftshader_indirect; use "off" if the emulator exits.

.PARAMETER ShowWindow
    Run with a real emulator window instead of -no-window. Some hosts cannot boot
    the guest headless; an interactive desktop session usually can.

.EXAMPLE
    pwsh -File scripts/verify-16kb-emulator.ps1

.EXAMPLE
    pwsh -File scripts/verify-16kb-emulator.ps1 -Port 5558 -BootTimeoutSec 900 -GpuMode off
#>
param(
    [int]$Port = 5556,
    [int]$BootTimeoutSec = 600,
    [string]$GpuMode = "swiftshader_indirect",
    [switch]$ShowWindow,
    [string]$Apk = "",
    [string]$TestApk = ""
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

function Write-Step([string]$Message) {
    Write-Host ("[{0}] {1}" -f (Get-Date -Format "HH:mm:ss"), $Message)
}

# Runs adb without ever turning its stderr ("device offline") into a terminating
# error. Returns the combined output and the exit code.
function Invoke-Adb {
    param([string[]]$AdbArgs)
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & $script:Adb @AdbArgs 2>&1
        $code = $LASTEXITCODE
        return [pscustomobject]@{ Text = (($output | Out-String).Trim()); Code = $code }
    }
    finally {
        $ErrorActionPreference = $previous
    }
}

$repo = Split-Path -Parent $PSScriptRoot
if (-not $Apk)     { $Apk     = Join-Path $repo "app\build\outputs\apk\debug\app-debug.apk" }
if (-not $TestApk) { $TestApk = Join-Path $repo "app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk" }

$sdk = $env:ANDROID_HOME
if (-not $sdk) { $sdk = $env:ANDROID_SDK_ROOT }
if (-not $sdk) { $sdk = Join-Path $env:LOCALAPPDATA "Android\Sdk" }

$script:Adb   = Join-Path $sdk "platform-tools\adb.exe"
$emulator     = Join-Path $sdk "emulator\emulator.exe"
$avdmanager   = Join-Path $sdk "cmdline-tools\latest\bin\avdmanager.bat"
$image        = "system-images;android-36;google_apis_ps16k;x86_64"
$avd          = "p16k"
$serial       = "emulator-$Port"
$logPath      = Join-Path $env:TEMP "p16k-emulator-$Port.log"

foreach ($p in @($script:Adb, $emulator)) {
    if (-not (Test-Path $p)) { throw "missing: $p (set ANDROID_HOME)" }
}
foreach ($p in @($Apk, $TestApk)) {
    if (-not (Test-Path $p)) { throw "missing APK: $p (run ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest first)" }
}

if (-not (Test-Path -LiteralPath $avdmanager)) { throw "missing avdmanager: $avdmanager (install the cmdline-tools package)" }

$avdIni = Join-Path $env:USERPROFILE ".android\avd\$avd.ini"
if (-not (Test-Path $avdIni)) {
    Write-Step "creating AVD $avd from $image"
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    & $avdmanager create avd -n $avd -k $image -d pixel_tablet --force 2>&1 | Out-Host
    $ErrorActionPreference = $prev
    if (-not (Test-Path $avdIni)) { throw "avdmanager did not create $avdIni" }
}

# Reuse an emulator that is already running on this port instead of starting a
# second one (a previous interrupted run leaves it behind).
$emu = $null
$devices = (Invoke-Adb -AdbArgs @("devices")).Text
$running = $devices -match [regex]::Escape($serial)
if ($running) {
    Write-Step "reusing the emulator already listening as $serial"
}
else {
    $emuArgs = @("-avd", $avd, "-port", "$Port", "-no-audio", "-no-boot-anim",
                 "-no-snapshot", "-gpu", $GpuMode, "-no-metrics")
    if (-not $ShowWindow) { $emuArgs += "-no-window" }
    Write-Step ("starting emulator {0} on port {1} (gpu {2}, window {3})" -f $avd, $Port, $GpuMode, [bool]$ShowWindow)
    $emu = Start-Process -FilePath $emulator -PassThru -RedirectStandardOutput $logPath -RedirectStandardError "$logPath.err" -ArgumentList $emuArgs
}

$exitCode = 1
try {
    $deadline = (Get-Date).AddSeconds($BootTimeoutSec)
    $booted = $false
    $lastReport = Get-Date
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
        if ($emu -and $emu.HasExited) {
            throw "the emulator exited early (code $($emu.ExitCode)); see $logPath"
        }
        $boot = Invoke-Adb -AdbArgs @("-s", $serial, "shell", "getprop", "sys.boot_completed")
        if ($boot.Code -eq 0 -and $boot.Text.Trim() -eq "1") { $booted = $true; break }
        if (((Get-Date) - $lastReport).TotalSeconds -ge 15) {
            $state = (Invoke-Adb -AdbArgs @("devices")).Text -split "`n" | Where-Object { $_ -match [regex]::Escape($serial) }
            if (-not $state) { $state = "(not visible to adb yet)" }
            Write-Step ("waiting for the guest to boot: {0}" -f $state)
            $lastReport = Get-Date
        }
    }
    if (-not $booted) {
        $state = (Invoke-Adb -AdbArgs @("devices")).Text
        $hint = "if it stays 'offline', retry with -ShowWindow (some hosts cannot boot headless) or -GpuMode off"
        throw "the guest did not boot within $BootTimeoutSec s. adb status:$([Environment]::NewLine)$state$([Environment]::NewLine)$hint$([Environment]::NewLine)see $logPath"
    }

    $page = (Invoke-Adb -AdbArgs @("-s", $serial, "shell", "getconf", "PAGE_SIZE")).Text.Trim()
    Write-Step "PAGE_SIZE = $page"
    if ($page -ne "16384") {
        throw "this is NOT a 16 KB page environment (PAGE_SIZE=$page); check that the AVD uses $image"
    }
    $sdkLevel = (Invoke-Adb -AdbArgs @("-s", $serial, "shell", "getprop", "ro.build.version.sdk")).Text.Trim()
    Write-Step "android sdk = $sdkLevel (expect 36)"

    Write-Step "installing $Apk"
    $install = Invoke-Adb -AdbArgs @("-s", $serial, "install", "-r", $Apk)
    $install.Text | Out-Host
    if ($install.Text -notmatch "Success") { throw "app install failed" }

    Write-Step "installing $TestApk"
    $installTest = Invoke-Adb -AdbArgs @("-s", $serial, "install", "-r", $TestApk)
    $installTest.Text | Out-Host
    if ($installTest.Text -notmatch "Success") { throw "test APK install failed" }

    Write-Step "running the JNI smoke test"
    $instrument = Invoke-Adb -AdbArgs @("-s", $serial, "shell", "am", "instrument", "-w",
        "io.github.dsh.protonnode.debug.test/androidx.test.runner.AndroidJUnitRunner")
    $instrument.Text | Out-Host
    if ($instrument.Text -notmatch "OK \(1 test\)") { throw "instrumentation did not report OK (1 test)" }

    Write-Step "A12 PASSED on a 16 KB page-size Android environment"
    $exitCode = 0
}
catch {
    Write-Host ""
    Write-Host ("FAILED: " + $_.Exception.Message) -ForegroundColor Red
    if (Test-Path $logPath) {
        Write-Host "--- last emulator log lines ($logPath) ---"
        Get-Content -LiteralPath $logPath -Tail 15 | Out-Host
    }
}
finally {
    if ($emu -and -not $emu.HasExited) {
        Write-Step "stopping the emulator"
        Invoke-Adb -AdbArgs @("-s", $serial, "emu", "kill") | Out-Null
        Start-Sleep -Seconds 2
        if (-not $emu.HasExited) { try { $emu.Kill() } catch { } }
    }
}

exit $exitCode
