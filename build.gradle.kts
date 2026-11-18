// Root build script: only plugin versions live here, modules configure themselves.
//
// Toolchain contract (docs/TEST-EVIDENCE.md records the exact versions that
// were used to produce the evidence):
//   AGP 9.5.1 / Gradle 9.6.0 / JDK 17+ (built and verified on JDK 25).
plugins {
    alias(libs.plugins.android.application) apply false
}
