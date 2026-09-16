package io.github.cyancity.easyunlocker.data

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

data class HistoryEntry(
    val requestId: String,
    val at: Long,
    val item: String,
    val requester: String,
    val purpose: String,
    val target: String,
    val decision: String,
    /** 批准方式：biometric / password；拒绝与超时为空。 */
    val via: String = "",
    /** 交付字段：密码 / 备注 / 名单 / 证书。 */
    val field: String = "",
    /** 从请求出现在列表到处理掉的毫秒数。 */
    val tookMs: Long = 0,
    val commandLine: String = "",
)

class HistoryStore(context: Context) {
    private val file = File(context.filesDir, "history.json")

    fun load(): List<HistoryEntry> {
        if (!file.exists()) return emptyList()
        return runCatching {
            val arr = JSONArray(file.readText())
            buildList {
                for (i in 0 until arr.length()) {
                    val o = arr.getJSONObject(i)
                    add(
                        HistoryEntry(
                            requestId = o.optString("request_id"),
                            at = o.optLong("at"),
                            item = o.optString("item"),
                            requester = o.optString("requester"),
                            purpose = o.optString("purpose"),
                            target = o.optString("target"),
                            decision = o.optString("decision"),
                            via = o.optString("via"),
                            field = o.optString("field"),
                            tookMs = o.optLong("took_ms"),
                            commandLine = o.optString("command_line"),
                        )
                    )
                }
            }
        }.getOrDefault(emptyList())
    }

    fun add(entry: HistoryEntry): List<HistoryEntry> {
        val next = (listOf(entry) + load().filter { it.requestId != entry.requestId }).take(200)
        save(next)
        return next
    }

    private fun save(items: List<HistoryEntry>) {
        val arr = JSONArray()
        items.forEach { e ->
            arr.put(
                JSONObject()
                    .put("request_id", e.requestId)
                    .put("at", e.at)
                    .put("item", e.item)
                    .put("requester", e.requester)
                    .put("purpose", e.purpose)
                    .put("target", e.target)
                    .put("decision", e.decision)
                    .put("via", e.via)
                    .put("field", e.field)
                    .put("took_ms", e.tookMs)
                    .put("command_line", e.commandLine),
            )
        }
        file.writeText(arr.toString())
    }
}
