# Release checklist

A release is blocked until **every** row is checked with evidence in
[TEST-EVIDENCE.md](TEST-EVIDENCE.md). Nothing here is "should be fine" — each row
carries the command that proves it.

## Mandatory gates

| # | Gate | How it is proven | Phase |
|---|---|---|---|
| 1 | **16 KB page runtime (A12)** — the app *runs* on a 16 KB kernel, not merely links | 16 KB system image (`getconf PAGE_SIZE› = 16384) + instrumentation + tunnel self-test; result in the "P2 后必测" subsection of TEST-EVIDENCE | first pass right after P2, **re-run before release** |
| 2 | 16 KB alignment of every `.so› in the APK | `scripts/check-16kb.sh <apk>› (readelf LOAD + `zipalign -c -P 16›) | P1 (done) / every release |
| 3 | Signed release APK | `./gradlew :app:assembleRelease› with `keystore.properties› injected from CI secrets | P7 |
| 4 | `SHA256SUMS› | `sha256sum app/build/outputs/apk/release/*.apk› | P7 |
| 5 | SBOM | `bash scripts/sbom.sh > sbom.cdx.json› | P7 |
| 6 | No secrets in history | `gitleaks detect --redact› on the whole history (0 findings) | P8 |
| 7 | No known vulnerabilities | `cd core && govulncheck ./...› (0 high) | P8 |
| 8 | Keystore-encrypted tokens | manual: app storage contains no plaintext token | P8 |
| 9 | Logs are redacted | unit test + manual: `RecentLogs› has no e-mail local part and no token | P8 |
| 10 | LICENSE / NOTICE / SECURITY.md present and current | repo review | P7 |
| 11 | GPL-3.0-or-later compliance | source tag published together with the APK | P7 |

## Release commands

```bash
cd core && go vet ./... && go test ./... -count=1 -race
cd .. && ./gradlew :app:ktlintCheck :app:lintDebug :app:testDebugUnitTest
./gradlew :app:assembleRelease
bash scripts/check-16kb.sh app/build/outputs/apk/release/app-release-unsigned.apk
sha256sum app/build/outputs/apk/release/*.apk > SHA256SUMS
bash scripts/sbom.sh > sbom.cdx.json
```

## What must NOT be in a release

- keystore files, `keystore.properties›, tokens, session JSON, `state.json›;
- APKs built from a dirty tree (record `git describe --dirty› in the release notes);
- a build whose TEST-EVIDENCE gate row still says `NOT RUN›.
