// Gradle settings for the android-proton repository.
//
// Modules:
//   :app   Android application (Kotlin; UI arrives in P5, service in P4).
//   core/  Go module (gomobile library) that is *not* a Gradle module: it is
//          built into an AAR by P1 and consumed by :app as a local dependency.
pluginManagement {
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "android-proton"
include(":app")
