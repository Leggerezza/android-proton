<#
.SYNOPSIS
    Verify the app on a real 16 KB page-size Android environment (A12).

.DESCRIPTION
    Boots the Android 16 "16 KB page size" emulator image, proves the guest really
    runs on 16 KB pages (getconf PAGE_SIZE = 16384), installs the debug APK and
    the instrumentation test, and runs the JNI smoke test.

    Run this from an INTERACTIVE desktop session (Android Studio / a normal
    terminal). Emulators started from a service / non-interactive session can
    stay "offline" forever.

.EXAMPLE
    pwsh -File scripts/verify-16kb-emulator.ps1
#>
param(
    [int]$Port = 5556,
    [int]$BootTimeoutSec = 600,
    [string]$Apk = "",
    [string]$TestApk = ""
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
if (-not $Apk)     { $Apk     = Join-Path $repo "app\build\outputs\apk\debug\app-debug.apk" }
if (-not $TestApk) { $TestApk = Join-Path $repo "app\build\outputs\apk\androidTest\debug\app-debug-androidTest.apk" }

$sdk = $env:ANDROID_HOME
if (-not $sdk) { $sdk = $env:ANDROID_SDK_ROOT }
if (-not $sdk) { $sdk = Join-Path $env:LOCALAPPDATA "Android\Sdk" }

$adb      = Join-Path $sdk "platform-tools\adb.exe"
$emulator = Join-Path $sdk "emulator\emulator.exe"
$avdmanager = Join-Path $sdk "cmdline-tools\latest\bin\avdmanager.bat"
$image    = "system-images;android-36;google_apis_ps16k;x86_64"
$avd      = "p16k"
$serial   = "emulator-$Port"

foreach ($p in @($adb, $emulator)) {
    if (-not (Test-Path $p)) { throw "missing: $p (set ANDROID_HOME)" }
}
foreach ($p in @($Apk, $TestApk)) {
    if (-not (Test-Path $p)) { throw "missing APK: $p (run ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest first)" }
}

if (-not (Test-Path $avdmanager)) { throw "missing avdmanager: $avdmanager (install cmdline-tools)" }

$avdIni = Join-Path $env:USERPROFILE ".android\avd\$avd.ini"
if (-not (Test-Path $avdIni)) {
    Write-Host "creating AVD $avd from $image"
    & $avdmanager create avd -n $avd -k $image -d pixel_tablet --force
    if ($LASTEXITCODE -ne 0) { throw "avdmanager failed" }
}

Write-Host "starting emulator on port $Port (log: $env:TEMP\p16k-emulator.log)"
$log = Join-Path $env:TEMP "p16k-emulator.log"
$emu = Start-Process -FilePath $emulator -PassThru -RedirectStandardOutput $log -RedirectStandardError "$log.err" -ArgumentList @(
    "-avd", $avd, "-port", "$Port", "-no-window", "-no-audio", "-no-boot-anim",
    "-no-snapshot", "-gpu", "swiftshader_indirect", "-no-metrics"
)

try {
    $deadline = (Get-Date).AddSeconds($BootTimeoutSec)
    $booted = $false
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
        $boot = (& $adb -s $serial shell getprop sys.boot_completed 2>$null)
        if ($boot -and $boot.Trim() -eq "1") { $booted = $true; break }
        if ($emu.HasExited) { throw "emulator exited early, see $log" }
    }
    if (-not $booted) { throw "guest did not boot within $BootTimeoutSec s, see $log" }
    Write-Host "booted"

    $page = (& $adb -s $serial shell getconf PAGE_SIZE).Trim()
    Write-Host "PAGE_SIZE = $page"
    if ($page -ne "16384") { throw "the guest is NOT a 16 KB environment (PAGE_SIZE=$page)" }

    Write-Host "installing $Apk"
    & $adb -s $serial install -r $Apk | Out-Host
    Write-Host "installing $TestApk"
    & $adb -s $serial install -r $TestApk | Out-Host

    Write-Host "running instrumentation"
    $out = & $adb -s $serial shell am instrument -w "io.github.dsh.protonnode.debug.test/androidx.test.runner.AndroidJUnitRunner"
    $out | Out-Host
    if (($out -join "`n") -notmatch "OK \(1 test\)") { throw "instrumentation did not report OK (1 test)" }

    Write-Host ""
    Write-Host "A12 PASSED on a 16 KB page-size Android environment." -ForegroundColor Green
    exit 0
}
finally {
    if ($emu -and -not $emu.HasExited) {
        & $adb -s $serial emu kill 2>$null | Out-Null
        Start-Sleep -Seconds 2
        if (-not $emu.HasExited) { $emu.Kill() }
    }
}
