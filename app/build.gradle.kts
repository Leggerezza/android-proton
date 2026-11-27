import java.io.FileInputStream
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.ktlint)
    alias(libs.plugins.compose.compiler)
}

ktlint {
    // Keep the style gate strict: a style violation fails CI.
    ignoreFailures = false
    filter {
        exclude { it.file.path.contains("/build/") }
    }
}

// Release signing is injected (CI Secrets or a local keystore.properties).
// The file is git-ignored; without it the release build stays unsigned, so a
// clean checkout still builds.
val keystorePropertiesFile = rootProject.file("keystore.properties")
val keystoreProperties =
    Properties().apply {
        if (keystorePropertiesFile.exists()) {
            FileInputStream(keystorePropertiesFile).use { load(it) }
        }
    }

android {
    buildFeatures {
        compose = true
        buildConfig = true
    }
    namespace = "io.github.leggerezza.protonnode"
    compileSdk = 36
    // Pinned to the NDK that produced the P1 evidence (r28c, 16 KB ready).
    ndkVersion = "28.2.13676358"

    defaultConfig {
        applicationId = "io.github.leggerezza.protonnode"
        minSdk = 26
        targetSdk = 36
        versionCode = 10
        versionName = "0.1.9"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // 16 KB page devices (PROMPT.md section 6.1): one ABI only.
        ndk {
            abiFilters += listOf("arm64-v8a")
            // Test-only escape hatch: the Google 16 KB page-size emulator is
            // x86_64 and its arm64 translation aborts on some Go code (SIGILL in
            // libndk_translation), so A12 is checked with a natively built core:
            //   ./gradlew :app:assembleDebug -PemulatorAbi=x86_64
            providers.gradleProperty("emulatorAbi").orNull?.let { abiFilters += it }
        }
    }

    signingConfigs {
        if (keystorePropertiesFile.exists()) {
            create("release") {
                storeFile = file(keystoreProperties.getProperty("storeFile"))
                storePassword = keystoreProperties.getProperty("storePassword")
                keyAlias = keystoreProperties.getProperty("keyAlias")
                keyPassword = keystoreProperties.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            signingConfig = signingConfigs.findByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    // 16 KB page size support: never extract native libs (PROMPT.md 6.1).
    packaging {
        jniLibs {
            useLegacyPackaging = false
        }
    }

    lint {
        // Lint must stay green in CI; a real violation is a build failure.
        abortOnError = true
        checkDependencies = true
    }
}

dependencies {
    // The Go core, bound with gomobile (scripts/build-core-aar.sh). It is a
    // build artifact, not source: a fresh checkout has no AAR until the script
    // runs, so the dependency is conditional and the app still assembles.
    // arm64 for devices (core.aar), or the matching ABI for an emulator run
    // (core-x86_64.aar, produced by scripts/build-core-aar.sh with -target=android/amd64).
    val emulatorAbi = providers.gradleProperty("emulatorAbi").orNull
    implementation(libs.androidx.core.ktx)
    implementation(libs.appcompat)
    implementation(libs.activity.compose)
    implementation(libs.lifecycle.runtime.compose)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)
    implementation(libs.androidx.work.runtime.ktx)
    implementation(libs.kotlinx.coroutines.android)

    val coreAar = if (emulatorAbi == null) file("libs/core.aar") else file("libs/core-$emulatorAbi.aar")
    if (coreAar.exists()) {
        implementation(files(coreAar))
    } else if (emulatorAbi != null) {
        throw GradleException("missing $coreAar; build it with: gomobile bind -target=android/$emulatorAbi ...")
    }

    // Instrumentation smoke test: proves the gomobile bridge actually loads and
    // runs the Go runtime inside an app process on a device ("A1" in
    // docs/P0-AUDIT.md). No UI is involved.
    testImplementation(libs.junit)
    // Android ships org.json; the JVM unit tests need a real implementation.
    testImplementation("org.json:json:20240303")

    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.1")
}
