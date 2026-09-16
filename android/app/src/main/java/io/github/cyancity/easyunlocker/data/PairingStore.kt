package io.github.cyancity.easyunlocker.data

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

/**
 * 一个网关 = 一台 Broker + 这台手机在那里的设备令牌。
 *
 * 网关只决定「往哪儿发待批准 / 决定」，跟保险库没有关系：
 * 切换网关不会碰 vault.eu1，条目照旧。
 */
data class Pairing(
    val id: String,
    val name: String,
    val url: String,
    val deviceToken: String,
    val createdAt: Long = 0L,
)

class PairingStore(context: Context) {
    private val file = File(context.filesDir, "pairings.json")

    fun load(): List<Pairing> {
        if (!file.exists()) return emptyList()
        return runCatching {
            val arr = JSONArray(file.readText())
            buildList {
                for (i in 0 until arr.length()) {
                    val o = arr.getJSONObject(i)
                    val id = o.optString("id")
                    val url = o.optString("url")
                    if (id.isBlank() || url.isBlank()) continue
                    add(
                        Pairing(
                            id = id,
                            name = o.optString("name"),
                            url = url,
                            deviceToken = o.optString("device_token"),
                            createdAt = o.optLong("created_at"),
                        )
                    )
                }
            }
        }.getOrDefault(emptyList())
    }

    fun save(items: List<Pairing>) {
        val arr = JSONArray()
        items.forEach { p ->
            arr.put(
                JSONObject()
                    .put("id", p.id)
                    .put("name", p.name)
                    .put("url", p.url)
                    .put("device_token", p.deviceToken)
                    .put("created_at", p.createdAt),
            )
        }
        file.writeText(arr.toString())
    }
}

/** 比 URL 用的规范化：忽略大小写和结尾斜杠。 */
fun normalizeGatewayUrl(url: String): String = url.trim().trimEnd('/').lowercase()
