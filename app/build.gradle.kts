import java.io.FileInputStream
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.ktlint)
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
    namespace = "io.github.dsh.protonnode"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.dsh.protonnode"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"

        // 16 KB page devices (PROMPT.md section 6.1): one ABI only.
        ndk {
            abiFilters += listOf("arm64-v8a")
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
