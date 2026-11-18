package io.github.dsh.protonnode

import android.app.Application

/**
 * Process entry point of the node.
 *
 * P0 scope: this class exists so the Kotlin toolchain is exercised by
 * `./gradlew assembleDebug` before P1 adds the gomobile bridge. It
 * deliberately does nothing yet (no service, no UI, no network).
 */
class ProtonNodeApp : Application()
