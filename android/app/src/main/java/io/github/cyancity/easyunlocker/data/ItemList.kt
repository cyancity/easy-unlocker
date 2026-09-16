package io.github.cyancity.easyunlocker.data

import org.json.JSONArray
import org.json.JSONObject

/**
 * 「条目名名单」的线上形状，必须与 CLI 的 `cli/items.go`（ParseItemList）逐字对齐。
 *
 * 只有名字和别名，永远不含任何值。空库封回 `"items":[]`——CLI 靠 items 键在不在，
 * 区分「库里没有条目」和「这根本不是一份名单」。
 */
object ItemListWire {
    const val VERSION = 1

    fun encode(items: List<VaultItem>): ByteArray {
        val entries = JSONArray()
        items.forEach { item ->
            val aliases = JSONArray()
            item.aliases.forEach { aliases.put(it) }
            entries.put(
                JSONObject()
                    .put("name", item.name)
                    .put("aliases", aliases),
            )
        }
        return JSONObject()
            .put("v", VERSION)
            .put("items", entries)
            .toString()
            .toByteArray(Charsets.UTF_8)
    }
}
