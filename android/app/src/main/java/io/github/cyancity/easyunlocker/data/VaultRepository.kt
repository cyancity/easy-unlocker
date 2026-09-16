package io.github.cyancity.easyunlocker.data

import android.content.Context
import android.os.Environment
import io.github.cyancity.easyunlocker.crypto.KeystoreWrap
import io.github.cyancity.easyunlocker.crypto.PasswordWrap
import io.github.cyancity.easyunlocker.crypto.VaultCrypto
import io.github.cyancity.easyunlocker.crypto.VaultFile
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.UUID

class VaultRepository(private val context: Context) {
    private val file get() = File(context.filesDir, "vault.eu1")
    private val wrap = KeystoreWrap(File(context.filesDir, "vault.wrap"))
    private val passwordWrap = PasswordWrap(File(context.filesDir, "password.wrap"))
    private val recoveryInternal get() = File(context.filesDir, "recovery.txt")
    // 主线程（UI）与 bg 线程都会读，加 volatile 保证可见性。
    @Volatile
    private var key: ByteArray? = null

    @Volatile
    private var cache: VaultPlaintext? = null

    fun exists(): Boolean = file.exists()
    fun canFingerprint(): Boolean = wrap.hasWrap()
    fun recoveryFilePath(): String = publicRecoveryFile().absolutePath

    fun publicRecoveryFile(): File {
        val dir = context.getExternalFilesDir(Environment.DIRECTORY_DOCUMENTS) ?: context.filesDir
        return File(dir, "easy-unlocker-recovery.txt")
    }

    @Synchronized
    fun lock() {
        key?.fill(0)
        key = null
        cache = null
    }

    fun isUnlocked(): Boolean = key != null

    fun create(recoveryCode: String) {
        val salt = ByteArray(16).also { java.security.SecureRandom().nextBytes(it) }
        val recovery = VaultCrypto.normalizeRecovery(recoveryCode)
        val derived = VaultCrypto.deriveKey(recovery, salt, 3, 64 * 1024)
        recovery.fill(0)
        val fresh = VaultPlaintext(vaultId = UUID.randomUUID().toString())
        persist(derived, salt, 3, 64 * 1024, fresh)
        key = derived
        cache = fresh
        saveRecovery(recoveryCode)
        passwordWrap.clear()
        runCatching { wrap.wrap(derived) }
    }

    fun unlock(recoveryCode: String) {
        openWithDerived(unlockFromRecovery(recoveryCode))
        saveRecovery(recoveryCode)
        runCatching { wrap.wrap(key!!) }
    }

    fun unlockWithFingerprint() {
        if (wrap.hasWrap()) {
            openWithDerived(wrap.unwrap())
            return
        }
        val stored = readStoredRecovery() ?: error("还没有指纹封存，请用恢复码解锁一次")
        unlock(stored)
    }

    fun hasPassword(): Boolean = passwordWrap.hasWrap()

    /** 设/改解锁密码：只重包内存里的 vault key，不重加密条目，所以要先解锁。 */
    @Synchronized
    fun setPassword(password: String) {
        val k = key ?: error("locked")
        require(password.isNotBlank()) { "密码不能为空" }
        passwordWrap.wrap(k, password)
    }

    fun clearPassword() {
        passwordWrap.clear()
    }

    fun unlockWithPassword(password: String) {
        val derived = try {
            passwordWrap.unwrap(password)
        } catch (e: Exception) {
            error("密码不对")
        }
        openWithDerived(derived)
        runCatching { wrap.wrap(key!!) }
    }

    /**
     * 批准时的密码兜底：库里已有 vault key（已解锁），拿密码解一层 wrap 比对即可。
     * 常数时间比较，防侧信道；密码错返回 false 而不是抛错——批准页不该崩。
     */
    fun verifyPassword(password: String): Boolean {
        val k = key ?: return false
        if (!passwordWrap.hasWrap()) return false
        val unwrapped = try {
            passwordWrap.unwrap(password)
        } catch (e: Exception) {
            return false
        }
        val ok = java.security.MessageDigest.isEqual(unwrapped, k)
        unwrapped.fill(0)
        return ok
    }

    private fun unlockFromRecovery(recoveryCode: String): ByteArray {
        val parsed = readFile()
        val recovery = VaultCrypto.normalizeRecovery(recoveryCode)
        val derived = VaultCrypto.deriveKey(recovery, VaultCrypto.unb64(parsed.salt), parsed.ops, parsed.memKiB)
        recovery.fill(0)
        return derived
    }

    @Synchronized
    private fun openWithDerived(derived: ByteArray) {
        val parsed = readFile()
        val plain = VaultCrypto.decryptVault(derived, VaultCrypto.unb64(parsed.nonce), VaultCrypto.unb64(parsed.ciphertext))
        key = derived
        var decoded = decodePlain(String(plain, Charsets.UTF_8))
        // 老库没有 vault_id：解锁时补一个并落盘，之后配对就能声明租户。
        if (decoded.vaultId.isBlank()) {
            decoded = decoded.copy(vaultId = UUID.randomUUID().toString())
            save(decoded)
        }
        cache = decoded
    }

    /** 租户锚点（库 UUID）；锁着时拿不到，返回空串由 Broker 归 default 租户。 */
    fun vaultId(): String = cache?.vaultId ?: ""

    private fun saveRecovery(code: String) {
        recoveryInternal.writeText(code)
        publicRecoveryFile().writeText(code + "\n")
    }

    private fun readStoredRecovery(): String? {
        val candidates = listOf(recoveryInternal, publicRecoveryFile())
        return candidates.firstOrNull { it.exists() }?.readText()?.trim()?.takeIf { it.isNotEmpty() }
    }

    @Synchronized
    fun items(): List<VaultItem> = cache?.items ?: emptyList()

    /**
     * 批量导入：名称（忽略大小写）冲突的整条跳过，别名冲突的只丢那个别名。
     *
     * 例外：**同名且库里备注为空**时，把导入项的备注补上（其余字段不动）。
     * 老版本导入只收密码栏、丢掉 notes，靠这条不用删条目重导就能把备注补回来。
     */
    @Synchronized
    fun importEntries(entries: List<ImportCandidate>): ImportMergeResult {
        val vault = cache ?: error("locked")
        val taken = vault.items.flatMap { listOf(it.name) + it.aliases }.map { it.lowercase() }.toMutableSet()
        val byName = vault.items.associateBy { it.name.lowercase() }
        // 同一个用户名落在多条候选上时语义不唯一（BW 里很常见），整批都不记别名，交给人工手选。
        val shared = mutableSetOf<String>()
        val seen = mutableSetOf<String>()
        entries.forEach { entry ->
            entry.aliases.map { it.trim().lowercase() }.filter { it.isNotEmpty() }.forEach {
                if (!seen.add(it)) shared += it
            }
        }
        val next = vault.items.toMutableList()
        var imported = 0
        var skippedName = 0
        var droppedAliases = 0
        var backfilled = 0
        entries.forEach { entry ->
            val name = entry.name.trim()
            // 保留名与空名一样整条跳过：导入是批量动作，不该因为一条坏名字中断整批。
            if (name.isEmpty() || ReservedItem.isReserved(name) || (entry.secret.isBlank() && entry.note.isBlank())) {
                skippedName++
                return@forEach
            }
            val existing = byName[name.lowercase()]
            if (existing != null) {
                if (existing.note.isBlank() && entry.note.isNotBlank()) {
                    val at = next.indexOfFirst { it.id == existing.id }
                    if (at >= 0) {
                        next[at] = existing.copy(note = entry.note)
                        backfilled++
                        return@forEach
                    }
                }
                skippedName++
                return@forEach
            }
            if (name.lowercase() in taken) {
                skippedName++
                return@forEach
            }
            val wanted = entry.aliases.map { it.trim() }
                .filter { it.isNotEmpty() && it.lowercase() != name.lowercase() && !ReservedItem.isReserved(it) }
            val kept = wanted.filter { it.lowercase() !in shared && it.lowercase() !in taken }
            droppedAliases += wanted.size - kept.size
            next += VaultItem(UUID.randomUUID().toString(), name, kept, entry.secret, entry.note)
            taken += name.lowercase()
            kept.forEach { taken += it.lowercase() }
            imported++
        }
        if (imported > 0 || backfilled > 0) save(vault.copy(items = next))
        return ImportMergeResult(imported, skippedName, droppedAliases, backfilled)
    }

    /** 保留名（#items）不许被条目占用，否则 CLI 再也请求不到名单。 */
    private fun requireNotReserved(name: String) {
        require(!ReservedItem.isReserved(name)) { "${ReservedItem.LIST} 是保留名，不能用作条目名或别名" }
    }

    @Synchronized
    fun add(name: String, secret: String, note: String = "", aliases: List<String> = emptyList()) {
        val vault = cache ?: error("locked")
        val trimmed = name.trim()
        require(trimmed.isNotBlank()) { "名称不能为空" }
        requireNotReserved(trimmed)
        aliases.forEach { requireNotReserved(it) }
        require(secret.isNotBlank() || note.isNotBlank()) { "密码和备注至少填一个" }
        val names = vault.items.flatMap { listOf(it.name) + it.aliases }.map { it.lowercase() }.toMutableSet()
        require(trimmed.lowercase() !in names) { "名称或别名重复" }
        aliases.forEach { require(it.lowercase() !in names) { "名称或别名重复" } }
        val next = vault.copy(items = vault.items + VaultItem(UUID.randomUUID().toString(), trimmed, aliases.map { it.trim() }.filter { it.isNotEmpty() }, secret, note))
        save(next)
    }

    @Synchronized
    fun update(id: String, name: String, secret: String, note: String = "") {
        val vault = cache ?: error("locked")
        val trimmed = name.trim()
        require(trimmed.isNotBlank()) { "名称不能为空" }
        requireNotReserved(trimmed)
        require(secret.isNotBlank() || note.isNotBlank()) { "密码和备注至少填一个" }
        val taken = vault.items.filter { it.id != id }.flatMap { listOf(it.name) + it.aliases }.map { it.lowercase() }
        require(trimmed.lowercase() !in taken) { "名称或别名重复" }
        save(vault.copy(items = vault.items.map {
            if (it.id == id) it.copy(name = trimmed, secret = secret, note = note) else it
        }))
    }

    @Synchronized
    fun delete(id: String) {
        val vault = cache ?: error("locked")
        save(vault.copy(items = vault.items.filterNot { it.id == id }))
    }

    @Synchronized
    fun addAlias(id: String, alias: String) {
        val trimmed = alias.trim()
        if (trimmed.isEmpty()) return
        requireNotReserved(trimmed)
        val vault = cache ?: error("locked")
        val taken = vault.items.flatMap { listOf(it.name) + it.aliases }.map { it.lowercase() }
        require(trimmed.lowercase() !in taken) { "名称或别名重复" }
        save(vault.copy(items = vault.items.map {
            if (it.id == id) it.copy(aliases = it.aliases + trimmed) else it
        }))
    }

    fun matchOrNull(requestName: String): VaultItem? {
        val key = requestName.lowercase()
        val hits = items().filter { it.name.lowercase() == key || it.aliases.any { a -> a.lowercase() == key } }
        return hits.singleOrNull()
    }

    fun exportBytes(): ByteArray = file.readBytes()

    fun importBytes(raw: ByteArray, recoveryCode: String) {
        file.writeBytes(raw)
        // 换库等于换钥匙：旧密码包出来的还是上一个库的 key，必须作废。
        passwordWrap.clear()
        unlock(recoveryCode)
    }

    @Synchronized
    private fun save(next: VaultPlaintext) {
        val k = key ?: error("locked")
        val parsed = if (file.exists()) readFile() else VaultFile(salt = VaultCrypto.b64(ByteArray(16)), nonce = "", ciphertext = "")
        val salt = if (file.exists()) VaultCrypto.unb64(parsed.salt) else ByteArray(16).also { java.security.SecureRandom().nextBytes(it) }
        persist(k, salt, parsed.ops.takeIf { file.exists() } ?: 3, parsed.memKiB.takeIf { file.exists() } ?: 64 * 1024, next)
        cache = next
    }

    private fun persist(derived: ByteArray, salt: ByteArray, ops: Int, mem: Int, vault: VaultPlaintext) {
        val (nonce, ct) = VaultCrypto.encryptVault(derived, encodePlain(vault).toByteArray(Charsets.UTF_8))
        val obj = JSONObject()
            .put("v", 1)
            .put("kdf", "argon2id")
            .put("salt", VaultCrypto.b64(salt))
            .put("nonce", VaultCrypto.b64(nonce))
            .put("ciphertext", VaultCrypto.b64(ct))
            .put("ops", ops)
            .put("memKiB", mem)
        file.writeText(obj.toString())
    }

    private fun readFile(): VaultFile {
        val obj = JSONObject(file.readText())
        return VaultFile(
            v = obj.optInt("v", 1),
            kdf = obj.optString("kdf", "argon2id"),
            salt = obj.getString("salt"),
            nonce = obj.getString("nonce"),
            ciphertext = obj.getString("ciphertext"),
            ops = obj.optInt("ops", 3),
            memKiB = obj.optInt("memKiB", 64 * 1024),
        )
    }

    private fun encodePlain(vault: VaultPlaintext): String {
        val items = JSONArray()
        vault.items.forEach { item ->
            val aliases = JSONArray()
            item.aliases.forEach { aliases.put(it) }
            items.put(
                JSONObject()
                    .put("id", item.id)
                    .put("name", item.name)
                    .put("aliases", aliases)
                    .put("secret", item.secret)
                    .put("note", item.note)
            )
        }
        return JSONObject()
            .put("items", items)
            .put("vault_id", vault.vaultId)
            .toString()
    }

    private fun decodePlain(raw: String): VaultPlaintext {
        val obj = JSONObject(raw)
        val arr = obj.optJSONArray("items") ?: JSONArray()
        val items = buildList {
            for (i in 0 until arr.length()) {
                val it = arr.getJSONObject(i)
                val aliases = it.optJSONArray("aliases") ?: JSONArray()
                add(
                    VaultItem(
                        id = it.getString("id"),
                        name = it.getString("name"),
                        aliases = buildList { for (j in 0 until aliases.length()) add(aliases.getString(j)) },
                        secret = it.getString("secret"),
                        note = if (it.isNull("note")) "" else it.optString("note", ""),
                    )
                )
            }
        }
        return VaultPlaintext(items, vaultId = obj.optString("vault_id", ""))
    }
}
