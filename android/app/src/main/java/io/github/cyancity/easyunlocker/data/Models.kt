package io.github.cyancity.easyunlocker.data

/** 一条凭据里可以作为「放出值」的两栏；审批时二选一。 */
object VaultField {
    const val PASSWORD = "password"
    const val NOTE = "note"

    /** 默认放出非空的那一栏（密码优先）。两栏都空时也返回密码，调用方会拦下并提示。 */
    fun defaultFor(secret: String, note: String): String = when {
        secret.isNotBlank() -> PASSWORD
        note.isNotBlank() -> NOTE
        else -> PASSWORD
    }
}

/** 请求方希望怎么拿到值；只用于给用户展示后果。 */
object Delivery {
    /** 写到 target 指定的文件，内容会留在那台机器上。空串（老 CLI）等价这个。 */
    const val FILE = "file"

    /** 只交给请求方进程（环境变量 / 匿名 fd），用完即焚，不落盘。 */
    const val EPHEMERAL = "ephemeral"
}

/**
 * CLI 请求「列出条目名」用的保留条目名，与 `cli/items.go` 的 ReservedListItem 必须一致。
 *
 * 复用 mode=write 是刻意的：两个 Broker 都只认 sign|write，加新 mode 就要同时重部署
 * Go broker 与 CF worker。App 看到这个名字就走专用批准页，只回名字，不放任何值。
 */
object ReservedItem {
    const val LIST = "#items"

    /** 网关送来的请求名是不是「要名单」。 */
    fun isListRequest(item: String): Boolean = item.trim() == LIST

    /** 保留名不许被条目占用，否则 CLI 再也请求不到名单。 */
    fun isReserved(name: String): Boolean = name.trim().equals(LIST, ignoreCase = true)
}

data class VaultItem(
    val id: String,
    val name: String,
    val aliases: List<String> = emptyList(),
    /** 短值：token、密码。 */
    val secret: String,
    /** 长文本：SSH 私钥、整段 .env、说明。可空。 */
    val note: String = "",
)

data class VaultPlaintext(
    val items: List<VaultItem> = emptyList(),
    /** 租户锚点：库初始化时生成的 UUID。Broker 按它路由请求；导出/导入沿用，
     * 所以换机后新手机落在同一租户、把旧手机踢下线。 */
    val vaultId: String = "",
)

/** 待导入条目，含明文 secret/note；只允许停留在内存，不进 UiState。 */
data class ImportCandidate(
    val id: Int,
    val name: String,
    val aliases: List<String> = emptyList(),
    val secret: String,
    val note: String = "",
)

/** 勾选页展示用，不含 secret。 */
data class ImportOption(
    val id: Int,
    val name: String,
    val username: String,
)

data class ImportMergeResult(
    val imported: Int,
    val skippedName: Int,
    val droppedAliases: Int,
    /** 名称已存在、库里备注为空而导入项有备注：只补备注，不动其它字段。 */
    val backfilled: Int = 0,
)

/** 已配对的设备（不含令牌本体；令牌只在自己设备上）。 */
data class PairedDevice(
    val id: String,
    val name: String,
    val createdAt: String,
    val lastUsedAt: String,
    val expiresAt: String,
    val current: Boolean,
)

data class PendingRequest(
    val request_id: String = "",
    val item: String = "",
    val mode: String = "",
    val purpose: String = "",
    val ttl: Int = 0,
    val target: String = "",
    val requester: String = "",
    val expires_at: String = "",
    val state: String = "",
    val seal_public_key: String = "",
    val delivery: String = "",
    /** sign 请求才有：要被签进证书的 SSH 公钥（authorized_keys 行）。 */
    val public_key: String = "",
    /** sign 请求才有：证书 principal（要登录的用户名）。 */
    val ssh_user: String = "",
    /** sign 请求才有：证书有效期（秒）；0 = 用默认 300。 */
    val cert_ttl: Int = 0,
    val selectedItemId: String? = null,
    /** 本地选择：这次放出密码（password）还是备注（note）。不来自网关。 */
    val selectedField: String? = null,
    /** 本地记录：这条请求第一次出现在列表里的时刻（epoch 毫秒，算审批耗时用）。 */
    val firstSeenAt: Long = 0,
) {
    /** 审批卡片上的命令行（近似还原当时那条 CLI；命令本体不会上传）。 */
    val commandLine: String
        get() = when {
            ReservedItem.isListRequest(item) -> "easyGet list --refresh"
            mode == "sign" -> "easyGet ssh $item"
            delivery == Delivery.EPHEMERAL -> "easyGet env $item --exec"
            else -> "easyGet env $item --write-to $target"
        }

    fun remainingSeconds(nowMillis: Long = System.currentTimeMillis()): Int {
        if (expires_at.isNotBlank()) {
            val exp = runCatching { java.time.Instant.parse(expires_at) }.getOrNull()
            if (exp != null) {
                return ((exp.toEpochMilli() - nowMillis) / 1000L).toInt().coerceAtLeast(0)
            }
        }
        return ttl.coerceAtLeast(0)
    }
}
