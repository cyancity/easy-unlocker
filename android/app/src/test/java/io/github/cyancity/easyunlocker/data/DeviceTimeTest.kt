package io.github.cyancity.easyunlocker.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

class DeviceTimeTest {
    private fun localDate(instant: Instant): String =
        instant.atZone(ZoneId.systemDefault()).toLocalDate().toString()

    @Test
    fun parsesIso() {
        val stamp = "2027-03-16T21:42:27.070Z"
        assertEquals(localDate(Instant.parse(stamp)), deviceDate(stamp))
    }

    @Test
    fun parsesEpochMillis() {
        val millis = 1_805_233_347_000L
        val text = deviceDate(millis.toString())
        assertEquals(localDate(Instant.ofEpochMilli(millis)), text)
        assertFalse("毫秒时间戳不该原样显示", text == millis.toString())
    }

    @Test
    fun parsesEpochSeconds() {
        val seconds = 1_805_233_347L
        val text = deviceDate(seconds.toString())
        assertEquals(localDate(Instant.ofEpochSecond(seconds)), text)
        assertFalse("秒级时间戳不该原样显示", text == seconds.toString())
    }

    @Test
    fun blankStaysBlank() {
        assertEquals("", deviceDate(""))
        assertEquals("", deviceDate("   "))
    }

    @Test
    fun unparsableFallsBackToShortenedText() {
        assertEquals("garbage-in", deviceDate("garbage-input-here"))
    }
}
