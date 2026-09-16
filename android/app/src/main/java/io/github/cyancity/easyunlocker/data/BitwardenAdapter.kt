package io.github.cyancity.easyunlocker.data

import org.json.JSONObject

/**
 * Bitwarden 未加密导出（.json）适配器。
 *
 * 对齐语义和 Bitwarden 一样：条目名 **或** `login.username` 都能跟 easyGet 的请求名对得上，
 * 密码取 `login.password`、备注取 `notes`。导入后 username 落到别名上，所以
 * `VaultRepository.matchOrNull` 不用改就能「对得上就返回值」。
 *
 * 认两种类型：`type == 1`（登录，密码栏 + notes）与 `type == 2`（安全笔记，只有 notes）。
 * SSH 私钥、OCI config 这类通常以安全笔记存放，只有 notes 有内容。
 * `login.totp` / `fields` / `uris` 可能含明文秘密，一律不读，也不打日志。
 */
object BitwardenAdapter {
    /** 选文件时读入的上限；BW 导出通常远小于这个数。 */
    const val MAX_BYTES = 16 * 1024 * 1024

    private const val TYPE_LOGIN = 1
    private const val TYPE_SECURE_NOTE = 2

    data class Result(
        val entries: List<ImportCandidate>,
        val skippedOtherType: Int,
        val skippedNoSecret: Int,
        val skippedNoName: Int,
        val skippedDuplicate: Int,
    ) {
        /** 文件里登录 + 安全笔记条目总数（含被跳过的）。 */
        val candidateCount: Int get() = entries.size + skippedNoSecret + skippedNoName + skippedDuplicate

        val skippedTotal: Int get() = skippedOtherType + skippedNoSecret + skippedNoName + skippedDuplicate
    }

    fun parse(raw: String): Result {
        val root = runCatching { JSONObject(raw) }.getOrElse {
            throw IllegalArgumentException("不是 JSON 文件。请选 Bitwarden 导出的 .json。")
        }
        require(!root.optBoolean("encrypted", false)) {
            "这是加密导出。请在 Bitwarden 里选「.json（未加密）」重新导一次。"
        }
        val items = root.optJSONArray("items")
            ?: throw IllegalArgumentException("不是 Bitwarden 导出文件：里面没有 items。")

        val entries = mutableListOf<ImportCandidate>()
        // 只按名称去重（BW 允许同名条目，这里只留第一条）。用户名的歧义留给
        // VaultRepository.importEntries 一起判：同名用户名出现在多条上时整批都不记别名。
        val claimedNames = mutableSetOf<String>()
        var otherType = 0
        var noSecret = 0
        var noName = 0
        var duplicate = 0

        for (i in 0 until items.length()) {
            val obj = items.optJSONObject(i) ?: continue
            val type = obj.optInt("type", 0)
            if (type != TYPE_LOGIN && type != TYPE_SECURE_NOTE) {
                otherType++
                continue
            }
            val login = if (type == TYPE_LOGIN) obj.optJSONObject("login") else null
            val username = str(login, "username").trim()
            val name = str(obj, "name").trim().ifEmpty { username }
            if (name.isEmpty()) {
                noName++
                continue
            }
            // 密码/备注都原样保留：前后空格可能是密钥的一部分，只判空白。
            val password = str(login, "password")
            val note = str(obj, "notes")
            if (password.isBlank() && note.isBlank()) {
                noSecret++
                continue
            }
            val key = name.lowercase()
            if (key in claimedNames) {
                duplicate++
                continue
            }
            val aliases = listOf(username).filter { it.isNotEmpty() && it.lowercase() != key }
            entries += ImportCandidate(entries.size, name, aliases, password, note)
            claimedNames += key
        }
        return Result(entries, otherType, noSecret, noName, duplicate)
    }

    /** 取字符串；缺字段和 JSON null 都当空串（`optString` 会把 null 变成字面量 "null"）。 */
    private fun str(obj: JSONObject?, key: String): String {
        if (obj == null || obj.isNull(key)) return ""
        return obj.optString(key).orEmpty()
    }
}
