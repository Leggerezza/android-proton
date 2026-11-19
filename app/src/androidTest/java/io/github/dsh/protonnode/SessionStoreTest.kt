package io.github.dsh.protonnode

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import io.github.dsh.protonnode.data.SessionStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

/** The session must round-trip through the Android Keystore (PROMPT.md 0.5). */
@RunWith(AndroidJUnit4::class)
class SessionStoreTest {
    @Test
    fun roundTripAndClear() {
        val store = SessionStore(InstrumentationRegistry.getInstrumentation().targetContext)
        store.clear()
        assertNull(store.load())

        val session = """{"uid":"u1","access_token":"a1","refresh_token":"r1","device_name":"d1"}"""
        store.save(session)
        assertEquals(session, store.load())

        store.clear()
        assertNull(store.load())
    }
}
