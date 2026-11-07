package io.github.cyancity.easyunlocker.data

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * 请求方（CLI 的 --field）指定的栏怎么影响默认选择。
 *
 * 老规矩：有密码放密码，没密码放备注。请求方点名了栏就以它为准——
 * 值存在备注里的条目不必每次都让用户手切。
 */
class VaultFieldTest {
    @Test
    fun defaultPrefersPassword() {
        assertEquals(VaultField.PASSWORD, VaultField.defaultFor("pw", "note"))
    }

    @Test
    fun defaultFallsBackToNoteWhenPasswordBlank() {
        assertEquals(VaultField.NOTE, VaultField.defaultFor("", "note"))
        assertEquals(VaultField.NOTE, VaultField.defaultFor("   ", "note"))
    }

    @Test
    fun defaultIsPasswordWhenBothBlank() {
        // 两栏都空时也返回密码，调用方会拦下并提示。
        assertEquals(VaultField.PASSWORD, VaultField.defaultFor("", ""))
    }

    @Test
    fun requestedFieldWins() {
        assertEquals(VaultField.NOTE, VaultField.requestedOrDefault(VaultField.NOTE, "pw", "note"))
        assertEquals(VaultField.PASSWORD, VaultField.requestedOrDefault(VaultField.PASSWORD, "pw", "note"))
    }

    @Test
    fun requestedFieldAppliesEvenWhenItIsTheBlankOne() {
        // 请求方点名了备注，但备注是空的：照样选备注，由批准页提示「这条没有备注」。
        // 不能悄悄换成密码——那不是请求方要的东西。
        assertEquals(VaultField.NOTE, VaultField.requestedOrDefault(VaultField.NOTE, "pw", ""))
    }

    @Test
    fun emptyRequestUsesOldDefault() {
        assertEquals(VaultField.PASSWORD, VaultField.requestedOrDefault("", "pw", "note"))
        assertEquals(VaultField.NOTE, VaultField.requestedOrDefault("", "", "note"))
    }

    @Test
    fun unknownRequestValueUsesOldDefault() {
        assertEquals(VaultField.PASSWORD, VaultField.requestedOrDefault("secret", "pw", "note"))
        assertEquals(VaultField.NOTE, VaultField.requestedOrDefault("stdout", "", "note"))
    }
}
