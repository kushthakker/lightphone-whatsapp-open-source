package org.lp3bridge.whatsapp

import kotlin.test.Test
import kotlin.test.assertFalse

class ToolEntryPointTest {
    @Test
    fun `does not enable remote push notifications`() {
        assertFalse(ToolEntryPoint.enablePushNotifications)
    }
}
