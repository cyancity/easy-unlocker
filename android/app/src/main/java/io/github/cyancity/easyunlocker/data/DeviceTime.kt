package io.github.cyancity.easyunlocker.data

import java.time.Instant
import java.time.ZoneId

/**
 * 设备令牌的时间戳 → 本地日期。
 *
 * 两台 Broker 都该发 RFC3339（Go 一直如此，CF worker 2026-09 才对齐），但 Broker 是自托管的，
 * App 与 Broker 各自升级——没重新部署的 worker 会发 epoch 毫秒。这里把 ISO / 秒 / 毫秒都认下来，
 * 认不出才退化成原文，别再把裸数字摆到用户眼前。
 */
fun deviceDate(stamp: String): String {
    val text = stamp.trim()
    if (text.isEmpty()) return ""
    val epoch = text.toLongOrNull()
    if (epoch != null) {
        // 秒级约 1e9，毫秒级约 1e12：按量级区分，免得把秒当毫秒显示成 1970 年
        val millis = if (epoch < 100_000_000_000L) epoch * 1000 else epoch
        return runCatching { localDate(Instant.ofEpochMilli(millis)) }.getOrDefault(text)
    }
    return runCatching { localDate(Instant.parse(text)) }.getOrDefault(text.take(10))
}

private fun localDate(instant: Instant): String =
    instant.atZone(ZoneId.systemDefault()).toLocalDate().toString()
