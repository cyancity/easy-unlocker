package io.github.cyancity.easyunlocker

import android.app.Application
import android.os.Handler
import android.os.Looper
import androidx.lifecycle.AndroidViewModel
import io.github.cyancity.easyunlocker.crypto.BoxPayload
import io.github.cyancity.easyunlocker.crypto.OpenSshKey
import io.github.cyancity.easyunlocker.crypto.SshCert
import io.github.cyancity.easyunlocker.crypto.VaultCrypto
import io.github.cyancity.easyunlocker.data.BitwardenAdapter
import io.github.cyancity.easyunlocker.data.BrokerClient
import io.github.cyancity.easyunlocker.data.HistoryEntry
import io.github.cyancity.easyunlocker.data.HistoryStore
import io.github.cyancity.easyunlocker.data.ImportCandidate
import io.github.cyancity.easyunlocker.data.ImportMergeResult
import io.github.cyancity.easyunlocker.data.ImportOption
import io.github.cyancity.easyunlocker.data.ItemListWire
import io.github.cyancity.easyunlocker.data.PairedDevice
import io.github.cyancity.easyunlocker.data.Pairing
import io.github.cyancity.easyunlocker.data.PairingStore
import io.github.cyancity.easyunlocker.data.PendingRequest
import io.github.cyancity.easyunlocker.data.ReservedItem
import io.github.cyancity.easyunlocker.data.VaultField
import io.github.cyancity.easyunlocker.data.VaultItem
import io.github.cyancity.easyunlocker.data.VaultRepository
import io.github.cyancity.easyunlocker.data.deviceDate
import io.github.cyancity.easyunlocker.data.normalizeGatewayUrl
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.UUID

enum class Screen { Setup, Unlock, Vault, Item, Edit, Pair, Pairings, Settings, Pending, Approved, History, HistoryDetail, Import, Devices }

/** 别的网关上有几条待批准。只报数量——批准必须在收到请求的那台网关上做，所以按钮是「切过去」。 */
data class OtherPending(val pairingId: String, val count: Int)

/** 其它网关的待批准多久查一次。它们只是角标，没必要跟当前网关一样 3 秒一轮。 */
private const val OTHER_POLL_INTERVAL_MS = 10_000L

data class UiState(
    val screen: Screen = Screen.Unlock,
    val recoveryPreview: String = "",
    val message: String = "",
    val toast: String = "",
    val formError: String = "",
    val items: List<VaultItem> = emptyList(),
    val pending: List<PendingRequest> = emptyList(),
    val selectedPending: PendingRequest? = null,
    val rememberAlias: Boolean = true,
    val openItemId: String? = null,
    val editingId: String? = null,
    val lastApprovedItem: String = "",
    val lastApprovedRequester: String = "",
    val pairings: List<Pairing> = emptyList(),
    val activePairingId: String = "",
    val canFingerprint: Boolean = false,
    val hasPassword: Boolean = false,
    val recoveryPath: String = "",
    val incoming: Boolean = false,
    val fromNotification: Boolean = false,
    val loading: Boolean = false,
    val offline: Boolean = false,
    val lastSync: String = "",
    val timedOut: Boolean = false,
    val haptics: Boolean = true,
    val sound: Boolean = true,
    val lastExportAt: String = "",
    /** 已选好备份文件、等输恢复码；备份字节在 VM 里，不进 state。 */
    val vaultImportPrompt: Boolean = false,
    val autoCloseSeconds: Int = 3,
    val history: List<HistoryEntry> = emptyList(),
    val openHistoryId: String? = null,
    val importOptions: List<ImportOption> = emptyList(),
    val importSelected: Set<Int> = emptySet(),
    val importNotice: String = "",
    val lastImportAt: String = "",
    val devicesByPairing: Map<String, List<PairedDevice>> = emptyMap(),
    val devicesErrorByPairing: Map<String, String> = emptyMap(),
    val devicesLoading: Boolean = false,
    val otherPending: List<OtherPending> = emptyList(),
    val pairCode: String = "",
    val pairCodeExpiresAt: Long = 0,
    /** 每 +1 表示「把过期的推送通知收掉」；MainActivity 监听它调 Notifications.clearAll。 */
    val clearNotifications: Int = 0,
) {
    /** 当前网关；空 = 未配对。UI 只关心这个，不关心列表本身。 */
    val activePairing: Pairing? get() = pairings.firstOrNull { it.id == activePairingId }
    /** 当前网关的设备。按网关存，所以换网关时它立刻跟着换，不会留着上一台的。 */
    val devices: List<PairedDevice> get() = devicesByPairing[activePairingId].orEmpty()
    /** 当前网关 + 其它网关的待批准总数：底部 tab 上的角标。 */
    val pendingBadge: Int get() = pending.size + otherPending.sumOf { it.count }
    val brokerUrl: String get() = activePairing?.url.orEmpty()
    val deviceName: String get() = activePairing?.name.orEmpty()
    val paired: Boolean get() = activePairing != null
}

class AppViewModel(app: Application) : AndroidViewModel(app) {
    private val repo = VaultRepository(app)
    private val prefs = Prefs(app)
    private val historyStore = HistoryStore(app)
    private val pairingStore = PairingStore(app)
    private val main = Handler(Looper.getMainLooper())
    private val _state = MutableStateFlow(initial())
    val state = _state.asStateFlow()
    private var watching = false
    private var vaultHold = 0
    /** 其它网关的待批准轮询：比主循环慢一档，且同一时刻只允许一个在跑。 */
    private var lastOtherPollAt = 0L
    private var otherPolling = false

    /** 最近一次拿到的 FCM 令牌：加新网关时也要给它注册一份。 */
    private var pushToken: String = ""

    /** 待导入条目（含明文密码）。只活在内存里，绝不进 UiState / 日志 / 文件。 */
    private var pendingImport: List<ImportCandidate> = emptyList()
    private val watch = object : Runnable {
        override fun run() {
            if (!watching) return
            silentRefresh()
            val now = System.currentTimeMillis()
            if (now - lastOtherPollAt >= OTHER_POLL_INTERVAL_MS) {
                lastOtherPollAt = now
                pollOtherGateways()
            }
            val wait = if (_state.value.pending.isNotEmpty()) 1000L else 3000L
            main.postDelayed(this, wait)
        }
    }

    fun startWatching() {
        if (_state.value.activePairing != null && !watching) {
            watching = true
            // 回前台先查一次别的网关，否则最多要等一个轮询间隔才知道那边有请求
            lastOtherPollAt = 0L
            main.post(watch)
        }
    }

    fun stopWatching() {
        watching = false
        main.removeCallbacks(watch)
    }

    /** FCM 令牌是设备级的：每台配对的网关都要注册一份，谁都能推你。 */
    fun registerPushToken(token: String) {
        if (token.isBlank()) return
        pushToken = token
        val targets = _state.value.pairings.filter { it.deviceToken.isNotBlank() }
        if (targets.isEmpty()) return
        bg {
            targets.forEach { pairing ->
                runCatching { BrokerClient(pairing.url, pairing.deviceToken).registerPushToken(token) }
            }
        }
    }

    /** 请求已经没意义了（批过 / 过期 / 撤销）：让 MainActivity 把通知栏里那条也收掉。 */
    private fun dropStaleNotifications() {
        _state.value = _state.value.copy(clearNotifications = _state.value.clearNotifications + 1)
    }

    /** 没填名字时拿 host 当网关名。 */
    private fun gatewayLabel(url: String): String =
        url.removePrefix("https://").removePrefix("http://").trimEnd('/').ifBlank { "网关" }

    /** 当前网关的客户端；没配对返回 null。 */
    private fun client(): BrokerClient? =
        _state.value.activePairing?.let { BrokerClient(it.url, it.deviceToken) }

    private fun loadPairings(): List<Pairing> {
        val stored = pairingStore.load()
        if (stored.isNotEmpty()) {
            val active = prefs.activePairingId
            if (stored.none { it.id == active }) prefs.activePairingId = stored.first().id
            return stored
        }
        // 一次性迁移：单网关时代留下的三个字段
        val legacyToken = prefs.deviceToken
        val legacyUrl = prefs.brokerUrl
        if (legacyToken.isBlank() || legacyUrl.isBlank()) return emptyList()
        val migrated = Pairing(
            id = UUID.randomUUID().toString(),
            name = prefs.deviceName.ifBlank { gatewayLabel(legacyUrl) },
            url = legacyUrl,
            deviceToken = legacyToken,
            createdAt = System.currentTimeMillis(),
        )
        pairingStore.save(listOf(migrated))
        prefs.activePairingId = migrated.id
        prefs.clearLegacyPairing()
        return listOf(migrated)
    }

    private fun bg(block: () -> Unit) {
        Thread {
            try {
                block()
            } catch (e: Exception) {
                main.post {
                    releaseVault()
                    _state.value = _state.value.copy(
                        message = e.message ?: "失败",
                        toast = e.message ?: "失败",
                        loading = false,
                    )
                }
            }
        }.start()
    }

    private fun initial(): UiState {
        val screen = when {
            !repo.exists() -> Screen.Setup
            else -> Screen.Unlock
        }
        return UiState(
            screen = screen,
            recoveryPreview = if (screen == Screen.Setup) VaultCrypto.newRecoveryCode() else "",
            pairings = loadPairings(),
            activePairingId = prefs.activePairingId,
            canFingerprint = repo.canFingerprint(),
            hasPassword = repo.hasPassword(),
            recoveryPath = repo.recoveryFilePath(),
            haptics = prefs.haptics,
            sound = prefs.sound,
            lastExportAt = prefs.lastExportAt,
            autoCloseSeconds = prefs.autoCloseSeconds,
            history = historyStore.load(),
            lastImportAt = prefs.lastImportAt,
        )
    }

    /**
     * 通知点进来。推送里带了 gateway（网关公网地址）时先切到对应网关，
     * 否则保留当前；旧 Broker 不带这个字段，行为不变。
     */
    fun markFromNotification(gateway: String? = null) {
        val target = gateway?.takeIf { it.isNotBlank() }?.let { g ->
            _state.value.pairings.firstOrNull { normalizeGatewayUrl(it.url) == normalizeGatewayUrl(g) }
        }
        val switched = target != null && target.id != _state.value.activePairingId
        if (switched && target != null) {
            prefs.activePairingId = target.id
        }
        _state.value = _state.value.copy(
            fromNotification = true,
            activePairingId = target?.id ?: _state.value.activePairingId,
            pending = if (switched) emptyList() else _state.value.pending,
            selectedPending = if (switched) null else _state.value.selectedPending,
        )
        if (switched) onActiveChanged()
    }

    fun lock() {
        if (vaultHold > 0) return
        repo.lock()
        clearImport()
        _state.value = _state.value.copy(
            screen = if (repo.exists()) Screen.Unlock else Screen.Setup,
            items = emptyList(),
            openItemId = null,
            editingId = null,
            formError = "",
            loading = false,
            message = if (_state.value.pending.isNotEmpty() || _state.value.fromNotification) "有请求待批准" else "",
            canFingerprint = repo.canFingerprint(),
            hasPassword = repo.hasPassword(),
            recoveryPath = repo.recoveryFilePath(),
        )
    }

    fun createVault() {
        val code = _state.value.recoveryPreview
        bg {
            repo.create(code)
            main.post { openVault(Screen.Vault) }
        }
    }

    fun unlock(code: String) {
        bg {
            repo.unlock(code)
            main.post { openVault() }
        }
    }

    fun unlockWithPassword(password: String) {
        bg {
            repo.unlockWithPassword(password)
            main.post { openVault() }
        }
    }

    /** 解锁密码：批准时的兜底认证。设/改只在已解锁状态下进行。 */
    fun setPassword(password: String) {
        bg {
            repo.setPassword(password)
            main.post {
                _state.value = _state.value.copy(hasPassword = true, toast = "解锁密码已设置")
            }
        }
    }

    fun clearPassword() {
        repo.clearPassword()
        _state.value = _state.value.copy(hasPassword = false, toast = "解锁密码已清除")
    }

    /** 批准兜底：密码校验过才走正常批准；错密码只提示，不动库。 */
    fun approveWithPassword(password: String) {
        bg {
            val ok = repo.verifyPassword(password)
            main.post {
                if (ok) approveSelected(via = "password") else _state.value = _state.value.copy(toast = "密码不对")
            }
        }
    }

    fun unlockWithFingerprint() {
        bg {
            repo.unlockWithFingerprint()
            main.post { openVault() }
        }
    }

    private fun openVault(preferred: Screen? = null) {
        val wantPending = preferred == Screen.Pending ||
            _state.value.fromNotification ||
            _state.value.pending.isNotEmpty()
        val screen = when {
            preferred == Screen.Pair || preferred == Screen.Pairings || preferred == Screen.Settings ||
                preferred == Screen.Edit || preferred == Screen.Item -> preferred
            wantPending -> Screen.Pending
        else -> Screen.Vault
        }
        _state.value = _state.value.copy(
            screen = screen,
            items = repo.items(),
            message = "",
            fromNotification = false,
            canFingerprint = repo.canFingerprint(),
            hasPassword = repo.hasPassword(),
            recoveryPath = repo.recoveryFilePath(),
        )
        if (screen == Screen.Pending) silentRefresh(forceOpen = true, showLoading = _state.value.pending.isEmpty())
        else if (_state.value.pending.isEmpty() && _state.value.activePairing != null) {
            // 解锁进来时没有待批准：通知栏里那条多半已经过期了，顺手收掉
            dropStaleNotifications()
        }
    }

    fun go(screen: Screen) {
        val loading = screen == Screen.Pending && _state.value.pending.isEmpty() && _state.value.activePairing != null
        _state.value = _state.value.copy(
            screen = screen,
            message = "",
            formError = "",
            timedOut = if (screen == Screen.Pending) _state.value.timedOut else false,
            loading = if (screen == Screen.Pending) loading else _state.value.loading,
        )
        if (screen == Screen.Pending) silentRefresh(forceOpen = true, showLoading = loading)
    }

    fun openItem(id: String) {
        _state.value = _state.value.copy(screen = Screen.Item, openItemId = id, formError = "")
    }

    fun openHistory(id: String) {
        _state.value = _state.value.copy(screen = Screen.HistoryDetail, openHistoryId = id)
    }

    fun startEdit(id: String?) {
        _state.value = _state.value.copy(screen = Screen.Edit, editingId = id, formError = "")
    }

    fun saveItem(name: String, secret: String, note: String = "") {
        val trimmed = name.trim()
        when {
            trimmed.isEmpty() -> {
                _state.value = _state.value.copy(formError = "给它一个名称。")
                return
            }
            secret.isBlank() && note.isBlank() -> {
                _state.value = _state.value.copy(formError = "密码和备注至少填一个。")
                return
            }
        }
        val editing = _state.value.editingId
        val taken = _state.value.items
            .filter { it.id != editing }
            .flatMap { listOf(it.name) + it.aliases }
            .map { it.lowercase() }
        if (trimmed.lowercase() in taken) {
            _state.value = _state.value.copy(formError = "这个名称或别名已经存在，库内必须唯一。")
            return
        }
        runCatching {
            if (editing == null) {
                repo.add(trimmed, secret, note)
                _state.value = _state.value.copy(
                    items = repo.items(),
                    screen = Screen.Vault,
                    formError = "",
                    toast = "已添加",
                )
            } else {
                repo.update(editing, trimmed, secret, note)
                _state.value = _state.value.copy(
                    items = repo.items(),
                    openItemId = editing,
                    screen = Screen.Item,
                    formError = "",
                    toast = "已保存",
                )
            }
        }.onFailure { _state.value = _state.value.copy(formError = it.message ?: "无法保存") }
    }

    fun clearFormError() {
        if (_state.value.formError.isNotBlank()) _state.value = _state.value.copy(formError = "")
    }

    fun addItem(name: String, secret: String, note: String = "") {
        saveItem(name, secret, note)
    }

    fun deleteItem(id: String) {
        runCatching {
            repo.delete(id)
            _state.value = _state.value.copy(
                items = repo.items(),
                screen = Screen.Vault,
                openItemId = if (_state.value.openItemId == id) null else _state.value.openItemId,
                toast = "已删除",
            )
        }.onFailure { _state.value = _state.value.copy(toast = it.message ?: "无法删除") }
    }

    fun exportVault(): ByteArray {
        val bytes = repo.exportBytes()
        val stamp = SimpleDateFormat("M月d日", Locale.CHINA).format(Date())
        prefs.lastExportAt = stamp
        _state.value = _state.value.copy(lastExportAt = stamp, toast = "已生成密文备份 · 需要恢复码才能解开")
        return bytes
    }

    /** 选好的 .eu1 备份字节，等 sheet 里输完恢复码再落盘。 */
    private var pendingVaultBytes: ByteArray? = null

    /** 文件已选好：先验下像个备份，再弹恢复码输入。 */
    fun onVaultBackupPicked(raw: ByteArray) {
        val looks = raw.size in 32..64 * 1024 * 1024 &&
            runCatching { String(raw, 0, raw.size.coerceAtMost(64), Charsets.UTF_8).contains("\"kdf\"") }.getOrDefault(false)
        if (!looks) {
            _state.value = _state.value.copy(toast = "不是 easy-unlocker 的密文备份")
            return
        }
        pendingVaultBytes = raw
        _state.value = _state.value.copy(vaultImportPrompt = true)
    }

    fun cancelVaultImport() {
        pendingVaultBytes = null
        _state.value = _state.value.copy(vaultImportPrompt = false)
    }

    /** 覆盖式导入：写入备份文件、作废旧密码包，再用这份备份的恢复码解锁。 */
    fun confirmVaultImport(code: String) {
        val raw = pendingVaultBytes ?: return
        bg {
            repo.importBytes(raw, code)
            main.post {
                pendingVaultBytes = null
                _state.value = _state.value.copy(vaultImportPrompt = false, toast = "备份已导入")
                openVault(Screen.Vault)
            }
        }
    }

    /** 解析 Bitwarden 未加密导出，解析成功后进勾选页。密码只留在内存，不进 UiState。 */
    fun beginImport(raw: String) {
        if (!repo.isUnlocked()) {
            _state.value = _state.value.copy(message = "库已锁上，请先解锁再导入。")
            return
        }
        _state.value = _state.value.copy(loading = true, message = "")
        Thread {
            val parsed = runCatching { BitwardenAdapter.parse(raw) }
            main.post {
                parsed
                    .onSuccess { result ->
                        if (!repo.isUnlocked()) {
                            // 解析期间库被锁上（比如被切到后台），别再进勾选页。
                            clearImport()
                            _state.value = _state.value.copy(loading = false, message = "库已锁上，请重新解锁后再导入。")
                        } else if (result.entries.isEmpty()) {
                            clearImport()
                            _state.value = _state.value.copy(
                                loading = false,
                                message = "没有可导入的条目。${importNotice(result)}",
                            )
                        } else {
                            pendingImport = result.entries
                            _state.value = _state.value.copy(
                                loading = false,
                                screen = Screen.Import,
                                importOptions = result.entries.map {
                                    ImportOption(it.id, it.name, it.aliases.firstOrNull().orEmpty())
                                },
                                importSelected = emptySet(),
                                importNotice = importNotice(result),
                                message = "",
                                formError = "",
                            )
                        }
                    }
                    .onFailure {
                        clearImport()
                        _state.value = _state.value.copy(loading = false, message = it.message ?: "导入失败")
                    }
            }
        }.start()
    }

    fun toggleImport(id: Int) {
        val picked = _state.value.importSelected
        _state.value = _state.value.copy(
            importSelected = if (id in picked) picked - id else picked + id,
        )
    }

    /** 全选/取消全选只作用于当前可见（搜索过滤后）的条目。 */
    fun toggleAllImport(visible: List<Int>) {
        if (visible.isEmpty()) return
        val picked = _state.value.importSelected
        val allOn = visible.all { it in picked }
        _state.value = _state.value.copy(
            importSelected = if (allOn) picked - visible.toSet() else picked + visible,
        )
    }

    fun confirmImport() {
        val picked = pendingImport.filter { it.id in _state.value.importSelected }
        if (picked.isEmpty()) {
            _state.value = _state.value.copy(toast = "先勾选要导入的条目")
            return
        }
        runCatching {
            require(repo.isUnlocked()) { "库已锁上，请重新解锁后再导入。" }
            repo.importEntries(picked)
        }.onSuccess { merged ->
            val stamp = SimpleDateFormat("M月d日", Locale.CHINA).format(Date())
            prefs.lastImportAt = stamp
            val toast = importResultText(merged)
            clearImport()
            _state.value = _state.value.copy(
                items = repo.items(),
                screen = Screen.Vault,
                openItemId = null,
                    formError = "",
                toast = toast,
                lastImportAt = stamp,
            )
        }.onFailure {
            clearImport()
            _state.value = _state.value.copy(
                screen = Screen.Settings,
                message = it.message ?: "导入失败",
            )
        }
    }

    fun cancelImport() {
        clearImport()
        _state.value = _state.value.copy(screen = Screen.Settings, message = "")
    }

    private fun clearImport() {
        pendingImport = emptyList()
        _state.value = _state.value.copy(
            importOptions = emptyList(),
            importSelected = emptySet(),
            importNotice = "",
        )
    }

    private fun importNotice(result: BitwardenAdapter.Result): String {
        val parts = mutableListOf("共 ${result.candidateCount} 条可导入条目")
        if (result.skippedOtherType > 0) parts += "${result.skippedOtherType} 条非登录/非安全笔记已忽略"
        if (result.skippedNoSecret > 0) parts += "${result.skippedNoSecret} 条密码和备注都是空的"
        if (result.skippedDuplicate > 0) parts += "${result.skippedDuplicate} 条重名"
        return parts.joinToString(" · ")
    }

    private fun importResultText(merged: ImportMergeResult): String {
        if (merged.imported == 0 && merged.backfilled == 0) {
            return "没有导入：名称都已存在且备注非空（跳过 ${merged.skippedName} 条）"
        }
        val parts = mutableListOf<String>()
        if (merged.imported > 0) parts += "已导入 ${merged.imported} 条"
        if (merged.backfilled > 0) parts += "${merged.backfilled} 条同名条目补上了备注"
        if (merged.skippedName > 0) parts += "跳过 ${merged.skippedName} 条重名"
        if (merged.droppedAliases > 0) parts += "${merged.droppedAliases} 个用户名和别的条目重名，没记成别名"
        return parts.joinToString(" · ")
    }

    /**
     * 添加或更新一台网关（不再覆盖已有的）。URL 已存在就更新那台的设备令牌（换 token 场景），
     * 否则新增一条并切过去。保险库不受影响。
     */
    fun pair(url: String, pairingToken: String, name: String) {
        val cleanUrl = url.trim().trimEnd('/')
        if (cleanUrl.isEmpty() || pairingToken.isBlank()) {
            _state.value = _state.value.copy(formError = "Broker 地址和 pairing token 都要填。")
            return
        }
        _state.value = _state.value.copy(loading = true, formError = "", message = "")
        Thread {
            runCatching {
                val label = name.trim().ifBlank { gatewayLabel(cleanUrl) }
                val deviceToken = BrokerClient(cleanUrl, "").pair(pairingToken, label, repo.vaultId())
                val existing = _state.value.pairings.firstOrNull {
                    normalizeGatewayUrl(it.url) == normalizeGatewayUrl(cleanUrl)
                }
                val entry = existing?.copy(name = label, url = cleanUrl, deviceToken = deviceToken)
                    ?: Pairing(UUID.randomUUID().toString(), label, cleanUrl, deviceToken, System.currentTimeMillis())
                val next = if (existing == null) _state.value.pairings + entry
                else _state.value.pairings.map { if (it.id == existing.id) entry else it }
                pairingStore.save(next)
                prefs.activePairingId = entry.id
                if (pushToken.isNotBlank()) {
                    runCatching { BrokerClient(cleanUrl, deviceToken).registerPushToken(pushToken) }
                }
                next to (existing == null)
            }.onSuccess { (next, added) ->
                main.post {
                    _state.value = _state.value.copy(
                        pairings = next,
                        activePairingId = prefs.activePairingId,
                        pending = emptyList(),
                        selectedPending = null,
                        loading = false,
                        formError = "",
                        toast = if (added) "已添加网关" else "已更新网关",
                        screen = Screen.Pairings,
                    )
                    onActiveChanged()
                    startWatching()
                }
            }.onFailure { error ->
                main.post {
                    _state.value = _state.value.copy(
                        loading = false,
                        formError = error.message ?: "配对失败",
                    )
                }
            }
        }.start()
    }

    /** 切换网关：只换「往哪儿发」，本地条目一点不动。旧网关的待批准请求属于它，先清掉。 */
    fun switchPairing(id: String) {
        applySwitch(id, openPending = false)
    }

    /** 「别的网关有待批准」的入口：切过去，并把那条请求直接展开，省一次点击。 */
    fun switchPairingForPending(id: String) {
        if (id == _state.value.activePairingId) {
            silentRefresh(forceOpen = true, showLoading = _state.value.pending.isEmpty())
            return
        }
        applySwitch(id, openPending = true)
    }

    private fun applySwitch(id: String, openPending: Boolean) {
        val target = _state.value.pairings.firstOrNull { it.id == id } ?: return
        if (id == _state.value.activePairingId) return
        prefs.activePairingId = id
        _state.value = _state.value.copy(
            activePairingId = id,
            pending = emptyList(),
            selectedPending = null,
            incoming = false,
            timedOut = false,
            offline = false,
            message = "",
            toast = "已切到 ${target.name}",
        )
        onActiveChanged()
        startWatching()
        silentRefresh(showLoading = true, allowAutoOpen = openPending)
    }

    /**
     * 当前网关变了。设备是按网关存的（`devicesByPairing`），所以「设备」那一行天然跟着换；
     * 这里只负责把新网关的设备拉一遍、并把它从「其它网关待批准」里摘掉。
     */
    private fun onActiveChanged() {
        _state.value = _state.value.copy(
            otherPending = _state.value.otherPending.filterNot { it.pairingId == _state.value.activePairingId },
        )
        loadDevices()
    }

    /** 改网关显示名。只改本地标签；Broker 那边记住的还是当初配对时的设备名。 */
    fun renamePairing(id: String, name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) {
            _state.value = _state.value.copy(toast = "名字不能为空")
            return
        }
        val next = _state.value.pairings.map { if (it.id == id) it.copy(name = trimmed) else it }
        pairingStore.save(next)
        _state.value = _state.value.copy(pairings = next, toast = "已改名")
    }

    fun removePairing(id: String) {
        val next = _state.value.pairings.filterNot { it.id == id }
        pairingStore.save(next)
        val wasActive = id == _state.value.activePairingId
        val nextActive = if (wasActive) (next.firstOrNull()?.id ?: "") else _state.value.activePairingId
        prefs.activePairingId = nextActive
        _state.value = _state.value.copy(
            pairings = next,
            activePairingId = nextActive,
            pending = if (wasActive) emptyList() else _state.value.pending,
            selectedPending = if (wasActive) null else _state.value.selectedPending,
            devicesByPairing = _state.value.devicesByPairing - id,
            devicesErrorByPairing = _state.value.devicesErrorByPairing - id,
            otherPending = _state.value.otherPending.filterNot { it.pairingId == id },
            toast = "已删除网关",
        )
        if (nextActive.isEmpty()) stopWatching() else if (wasActive) {
            onActiveChanged()
            startWatching()
            silentRefresh(allowAutoOpen = false)
        }
    }

    // ---------- 设备管理：配对码 / 设备列表 / 撤销 / 续期 ----------

    fun openDevices() {
        _state.value = _state.value.copy(
            screen = Screen.Devices,
            devicesLoading = true,
            pairCode = "",
            pairCodeExpiresAt = 0,
        )
        loadDevices()
    }

    /**
     * 拉所有已配网关的设备表：设备属于各自的 Broker，设备页要看全，设置页只显示当前那台的条数。
     *
     * 不走 `bg{}`——它的 catch 会 releaseVault 并弹全局错误，一台下线的网关不该影响整机。
     * 单台失败只记在那台的 error 上（401 = 那边的审批端已经失效）。
     */
    fun loadDevices() {
        val pairings = _state.value.pairings.filter { it.deviceToken.isNotBlank() }
        if (pairings.isEmpty()) {
            _state.value = _state.value.copy(
                devicesByPairing = emptyMap(),
                devicesErrorByPairing = emptyMap(),
                devicesLoading = false,
            )
            return
        }
        _state.value = _state.value.copy(devicesLoading = true)
        Thread {
            val lists = mutableMapOf<String, List<PairedDevice>>()
            val errors = mutableMapOf<String, String>()
            pairings.forEach { pairing ->
                runCatching { BrokerClient(pairing.url, pairing.deviceToken).devices() }
                    .onSuccess { lists[pairing.id] = it }
                    .onFailure { errors[pairing.id] = it.message ?: "加载设备失败" }
            }
            main.post {
                val alive = _state.value.pairings.map { it.id }.toSet()
                _state.value = _state.value.copy(
                    devicesByPairing = _state.value.devicesByPairing.filterKeys { it in alive } + lists,
                    devicesErrorByPairing = _state.value.devicesErrorByPairing.filterKeys { it in alive } + errors,
                    devicesLoading = false,
                )
            }
        }.start()
    }

    /** 生成一次性配对码：10 分钟有效，新机器 `easyGet pair --code <码>` 用。 */
    fun createPairCode() {
        val pairing = _state.value.activePairing ?: return
        _state.value = _state.value.copy(devicesLoading = true)
        bg {
            val result = runCatching { BrokerClient(pairing.url, pairing.deviceToken).createPairCode() }
            main.post {
                result.fold(
                    onSuccess = { (code, expiresAt) ->
                        _state.value = _state.value.copy(pairCode = code, pairCodeExpiresAt = expiresAt, devicesLoading = false)
                    },
                    onFailure = { e ->
                        _state.value = _state.value.copy(devicesLoading = false, message = e.message ?: "生成配对码失败")
                    },
                )
            }
        }
    }

    fun clearPairCode() {
        _state.value = _state.value.copy(pairCode = "", pairCodeExpiresAt = 0)
    }

    fun renewDevice() {
        val pairing = _state.value.activePairing ?: return
        bg {
            val result = runCatching {
                val client = BrokerClient(pairing.url, pairing.deviceToken)
                client.renewDevice() to client.devices()
            }
            main.post {
                result.fold(
                    onSuccess = { (expiresAt, list) ->
                        _state.value = _state.value.copy(
                            devicesByPairing = _state.value.devicesByPairing + (pairing.id to list),
                            toast = "已续期，有效期至 ${deviceDate(expiresAt)}",
                        )
                    },
                    onFailure = { e -> _state.value = _state.value.copy(message = e.message ?: "续期失败") },
                )
            }
        }
    }

    /** 给设备改名。改的是那台 Broker 上的设备名；设备页按网关分组，所以要带上是哪台网关。 */
    fun renameDevice(pairingId: String, id: String, name: String) {
        val pairing = _state.value.pairings.firstOrNull { it.id == pairingId } ?: return
        val trimmed = name.trim()
        if (trimmed.isEmpty()) {
            _state.value = _state.value.copy(toast = "名字不能为空")
            return
        }
        bg {
            val result = runCatching { BrokerClient(pairing.url, pairing.deviceToken).renameDevice(id, trimmed) }
            main.post {
                result.fold(
                    onSuccess = {
                        loadDevices()
                        _state.value = _state.value.copy(toast = "已改名")
                    },
                    onFailure = { e -> _state.value = _state.value.copy(message = e.message ?: "改名失败") },
                )
            }
        }
    }

    /**
     * 撤销一台设备（可以是别的网关上的）。撤销本机在那台的记录 = 从这台网点登出：
     * 那条配对记录同时清掉，否则 App 会留着一张已经失效的令牌。
     */
    fun revokeDevice(pairingId: String, id: String) {
        val pairing = _state.value.pairings.firstOrNull { it.id == pairingId } ?: return
        val isSelf = _state.value.devicesByPairing[pairingId]?.firstOrNull { it.id == id }?.current == true
        val wasActive = pairingId == _state.value.activePairingId
        bg {
            val result = runCatching { BrokerClient(pairing.url, pairing.deviceToken).revokeDevice(id) }
            main.post {
                result.fold(
                    onSuccess = {
                        if (isSelf) {
                            removePairing(pairing.id)
                            _state.value = _state.value.copy(
                                screen = if (wasActive) Screen.Settings else _state.value.screen,
                                toast = "已登出这台设备",
                            )
                        } else {
                            loadDevices()
                            _state.value = _state.value.copy(toast = "已撤销")
                        }
                    },
                    onFailure = { e -> _state.value = _state.value.copy(message = e.message ?: "撤销失败") },
                )
            }
        }
    }

    fun refreshPending() {
        silentRefresh(forceOpen = true, showLoading = _state.value.pending.isEmpty())
    }

    fun retryPending() {
        silentRefresh(forceOpen = true, showLoading = true)
    }

    /**
     * 别的网关有没有待批准。只数数量、只写 `otherPending`：一台网关连不上不该影响整机，
     * 所以这里既不走 `bg{}`（它的 catch 会 releaseVault + 弹全局错误），也不碰 offline。
     */
    private fun pollOtherGateways() {
        val activeId = _state.value.activePairingId
        val others = _state.value.pairings.filter { it.id != activeId && it.deviceToken.isNotBlank() }
        if (others.isEmpty()) {
            if (_state.value.otherPending.isNotEmpty()) {
                _state.value = _state.value.copy(otherPending = emptyList())
            }
            return
        }
        if (otherPolling) return
        otherPolling = true
        Thread {
            val found = buildList {
                others.forEach { pairing ->
                    val count = runCatching {
                        BrokerClient(pairing.url, pairing.deviceToken).pending().count { it.state == "waiting" }
                    }.getOrNull() ?: return@forEach
                    if (count > 0) add(OtherPending(pairing.id, count))
                }
            }
            main.post {
                otherPolling = false
                // 期间可能删了网关、或把它切成了当前网关（那就归主循环管）
                val alive = _state.value.pairings.map { it.id }.toSet()
                val nowActive = _state.value.activePairingId
                _state.value = _state.value.copy(
                    otherPending = found.filter { it.pairingId in alive && it.pairingId != nowActive },
                )
            }
        }.start()
    }

    private fun silentRefresh(
        forceOpen: Boolean = false,
        showLoading: Boolean = false,
        allowAutoOpen: Boolean = true,
    ) {
        val broker = client() ?: return
        val pairingId = _state.value.activePairingId
        if (showLoading) _state.value = _state.value.copy(loading = true, offline = false)
        bg {
            val list = try {
                broker.pending().filter { it.state == "waiting" }
            } catch (e: Exception) {
                main.post {
                    if (_state.value.activePairingId == pairingId) {
                        _state.value = _state.value.copy(loading = false, offline = true)
                    }
                }
                return@bg
            }
            val prev = _state.value.pending.associate {
                it.request_id to Triple(it.selectedItemId, it.selectedField, it.firstSeenAt)
            }
            val items = repo.items()
            val mapped = list.map { req ->
                val kept = prev[req.request_id]
                val itemId = kept?.first ?: repo.matchOrNull(req.item)?.id
                val item = itemId?.let { id -> items.firstOrNull { it.id == id } }
                req.copy(
                    selectedItemId = itemId,
                    selectedField = kept?.second ?: item?.let { VaultField.defaultFor(it.secret, it.note) },
                    firstSeenAt = kept?.third?.takeIf { it > 0 } ?: System.currentTimeMillis(),
                )
            }
            val currentId = _state.value.selectedPending?.request_id
            val selected = mapped.firstOrNull { it.request_id == currentId } ?: mapped.firstOrNull()
            main.post {
                // 请求飞行途中网关被切走了：这份结果属于旧网关，丢掉，别污染新网关的待批准
                if (_state.value.activePairingId != pairingId) return@post
                val prevIds = _state.value.pending.map { it.request_id }.toSet()
                val arrived = mapped.any { it.request_id !in prevIds }
                val open = forceOpen || (allowAutoOpen && arrived && mapped.isNotEmpty() && repo.isUnlocked())
                val lockedHint = arrived && mapped.isNotEmpty() && !repo.isUnlocked()
                val stamp = SimpleDateFormat("HH:mm", Locale.CHINA).format(Date())
                val vanished = prevIds.isNotEmpty() && mapped.isEmpty()
                val dropped = vanished && _state.value.screen == Screen.Pending
                if (dropped) {
                    _state.value.pending.forEach { rememberDecision(it, "expired") }
                }
                // 请求没了就把通知栏里那条一起收掉（不然会挂到用户手动划走）
                if (vanished) dropStaleNotifications()
                _state.value = _state.value.copy(
                    pending = mapped,
                    selectedPending = selected,
                    loading = false,
                    offline = false,
                    lastSync = stamp,
                    incoming = arrived && mapped.isNotEmpty(),
                    timedOut = if (dropped) true else _state.value.timedOut,
                    toast = if (dropped) "请求已结束" else _state.value.toast,
                    screen = if (open) Screen.Pending else _state.value.screen,
                    message = when {
                        lockedHint -> "有请求待批准，请先解锁"
                        forceOpen && mapped.isEmpty() && !_state.value.timedOut -> ""
                        else -> _state.value.message
                    },
                )
            }
        }
    }

    fun selectPending(req: PendingRequest) {
        _state.value = _state.value.copy(selectedPending = req)
    }

    fun selectItem(id: String) {
        val cur = _state.value.selectedPending ?: return
        val item = _state.value.items.firstOrNull { it.id == id }
        val next = cur.copy(
            selectedItemId = id,
            selectedField = item?.let { VaultField.defaultFor(it.secret, it.note) },
        )
        _state.value = _state.value.copy(
            pending = _state.value.pending.map { if (it.request_id == cur.request_id) next else it },
            selectedPending = next,
        )
    }

    /** 切换这次放出密码还是备注。 */
    fun selectField(field: String) {
        val cur = _state.value.selectedPending ?: return
        val next = cur.copy(selectedField = field)
        _state.value = _state.value.copy(
            pending = _state.value.pending.map { if (it.request_id == cur.request_id) next else it },
            selectedPending = next,
        )
    }

    fun setRememberAlias(value: Boolean) {
        _state.value = _state.value.copy(rememberAlias = value)
    }

    fun setHaptics(value: Boolean) {
        prefs.haptics = value
        _state.value = _state.value.copy(haptics = value)
    }

    fun setSound(value: Boolean) {
        prefs.sound = value
        _state.value = _state.value.copy(sound = value)
    }

    fun setAutoCloseSeconds(value: Int) {
        prefs.autoCloseSeconds = value
        _state.value = _state.value.copy(autoCloseSeconds = prefs.autoCloseSeconds)
    }

    /** 批准前的一次本地记账。via/field 只用于记录详情页展示，不进协议。 */
    private fun rememberDecision(
        req: PendingRequest,
        decision: String,
        itemName: String = req.item,
        via: String = "",
        field: String = "",
    ) {
        val now = System.currentTimeMillis()
        val next = historyStore.add(
            HistoryEntry(
                requestId = req.request_id,
                at = now,
                item = itemName,
                requester = req.requester,
                purpose = req.purpose,
                target = req.target,
                decision = decision,
                via = via,
                field = field,
                tookMs = if (req.firstSeenAt > 0) (now - req.firstSeenAt).coerceAtLeast(0) else 0,
                commandLine = req.commandLine,
            )
        )
        _state.value = _state.value.copy(history = next)
    }

    private fun holdVault() {
        vaultHold++
    }

    private fun releaseVault() {
        vaultHold = (vaultHold - 1).coerceAtLeast(0)
        if (vaultHold == 0 && !watching) lock()
    }

    fun clearIncoming() {
        if (_state.value.incoming) _state.value = _state.value.copy(incoming = false)
    }

    fun toast(msg: String) {
        _state.value = _state.value.copy(toast = msg)
    }

    fun clearToast() {
        _state.value = _state.value.copy(toast = "")
    }

    fun expireRequest(id: String) {
        val gone = _state.value.pending.firstOrNull { it.request_id == id } ?: return
        val left = _state.value.pending.filterNot { it.request_id == id }
        rememberDecision(gone, "expired")
        _state.value = _state.value.copy(
            pending = left,
            selectedPending = left.firstOrNull { it.request_id == _state.value.selectedPending?.request_id } ?: left.firstOrNull(),
            timedOut = true,
            toast = "已超时",
        )
        dropStaleNotifications()
    }

    /** via 只用于本地批准记录展示（biometric / password）。 */
    fun approveSelected(via: String = "biometric") {
        val pending = _state.value.selectedPending ?: return
        if (ReservedItem.isListRequest(pending.item)) {
            approveListRequest(pending, via)
            return
        }
        if (pending.mode == "sign") {
            approveSignRequest(pending, via)
            return
        }
        val itemId = pending.selectedItemId ?: run {
            _state.value = _state.value.copy(toast = "请先选一条凭据")
            return
        }
        if (!repo.isUnlocked()) {
            _state.value = _state.value.copy(toast = "库已锁上，请先解锁再批准")
            return
        }
        val item = repo.items().firstOrNull { it.id == itemId } ?: run {
            _state.value = _state.value.copy(toast = "找不到这条凭据")
            return
        }
        val field = pending.selectedField ?: VaultField.defaultFor(item.secret, item.note)
        val value = if (field == VaultField.NOTE) item.note else item.secret
        if (value.isBlank()) {
            _state.value = _state.value.copy(toast = if (field == VaultField.NOTE) "这条没有备注" else "这条没有密码")
            return
        }
        val broker = client() ?: run {
            _state.value = _state.value.copy(toast = "没有可用网关，请先添加")
            return
        }
        holdVault()
        bg {
            try {
                require(pending.seal_public_key.isNotBlank()) {
                    "这次请求没有传输公钥。用当前仓库重新编译的 ask 再发一次。"
                }
                val envelope = BoxPayload.seal(pending.seal_public_key, pending.request_id, value.toByteArray(Charsets.UTF_8))
                broker.decide(pending.request_id, "approve", envelope)
                if (_state.value.rememberAlias && pending.item.isNotBlank() &&
                    pending.item.lowercase() != item.name.lowercase() &&
                    pending.item.lowercase() !in item.aliases.map { it.lowercase() }
                ) {
                    runCatching { repo.addAlias(item.id, pending.item) }
                }
                main.post {
                    rememberDecision(pending, "approved", item.name, via, if (field == VaultField.NOTE) "备注" else "密码")
                    _state.value = _state.value.copy(
                        selectedPending = null,
                        pending = _state.value.pending.filterNot { it.request_id == pending.request_id },
                        items = repo.items(),
                        lastApprovedItem = item.name,
                        lastApprovedRequester = pending.requester,
                        screen = Screen.Approved,
                        timedOut = false,
                        toast = "",
                    )
                    dropStaleNotifications()
                    releaseVault()
                }
            } catch (e: Exception) {
                main.post {
                    releaseVault()
                    _state.value = _state.value.copy(toast = e.message ?: "批准失败", message = e.message ?: "批准失败")
                }
            }
        }
    }

    /**
     * 保留名请求：只把条目名和别名封回去，不放任何值。
     *
     * 与 CLI 的 `easyGet list --refresh` 配对，见 `cli/items.go` 与 `data/ItemList.kt`。
     */
    private fun approveListRequest(pending: PendingRequest, via: String) {
        if (!repo.isUnlocked()) {
            _state.value = _state.value.copy(toast = "库已锁上，请先解锁再批准")
            return
        }
        val broker = client() ?: run {
            _state.value = _state.value.copy(toast = "没有可用网关，请先添加")
            return
        }
        val items = repo.items()
        holdVault()
        bg {
            try {
                require(pending.seal_public_key.isNotBlank()) {
                    "这次请求没有传输公钥。用当前仓库重新编译的 easyGet 再发一次。"
                }
                val envelope = BoxPayload.seal(pending.seal_public_key, pending.request_id, ItemListWire.encode(items))
                broker.decide(pending.request_id, "approve", envelope)
                main.post {
                    rememberDecision(pending, "approved", ReservedItem.LIST, via, "名单")
                    _state.value = _state.value.copy(
                        selectedPending = null,
                        pending = _state.value.pending.filterNot { it.request_id == pending.request_id },
                        items = repo.items(),
                        lastApprovedItem = ReservedItem.LIST,
                        lastApprovedRequester = pending.requester,
                        screen = Screen.Approved,
                        timedOut = false,
                        toast = "",
                    )
                    dropStaleNotifications()
                    releaseVault()
                }
            } catch (e: Exception) {
                main.post {
                    releaseVault()
                    _state.value = _state.value.copy(toast = e.message ?: "批准失败", message = e.message ?: "批准失败")
                }
            }
        }
    }

    /**
     * sign 请求：选中的条目当 CA 用（它的密码/备注栏里要是一把 OpenSSH ed25519 私钥）。
     * 封回去的是证书本体——CA 私钥永远不会被放出。
     */
    private fun approveSignRequest(pending: PendingRequest, via: String) {
        val itemId = pending.selectedItemId ?: run {
            _state.value = _state.value.copy(toast = "先选一条当 CA 的条目")
            return
        }
        if (!repo.isUnlocked()) {
            _state.value = _state.value.copy(toast = "库已锁上，请先解锁再批准")
            return
        }
        val item = repo.items().firstOrNull { it.id == itemId } ?: run {
            _state.value = _state.value.copy(toast = "找不到这条凭据")
            return
        }
        val ca = OpenSshKey.parseOrNull(item.note) ?: OpenSshKey.parseOrNull(item.secret) ?: run {
            _state.value = _state.value.copy(toast = "这条不是 OpenSSH ed25519 私钥，换一条当 CA")
            return
        }
        val broker = client() ?: run {
            _state.value = _state.value.copy(toast = "没有可用网关，请先添加")
            return
        }
        holdVault()
        bg {
            try {
                require(pending.seal_public_key.isNotBlank()) {
                    "这次请求没有传输公钥。用当前仓库重新编译的 easyGet 再发一次。"
                }
                require(pending.public_key.isNotBlank() && pending.ssh_user.isNotBlank()) {
                    "这次 sign 请求没带公钥或用户名，没法签。"
                }
                val cert = SshCert.signUser(ca, pending.public_key, pending.ssh_user, pending.request_id, pending.cert_ttl)
                val envelope = BoxPayload.seal(pending.seal_public_key, pending.request_id, cert.toByteArray(Charsets.UTF_8))
                broker.decide(pending.request_id, "approve", envelope)
                if (_state.value.rememberAlias && pending.item.isNotBlank() &&
                    pending.item.lowercase() != item.name.lowercase() &&
                    pending.item.lowercase() !in item.aliases.map { it.lowercase() }
                ) {
                    runCatching { repo.addAlias(item.id, pending.item) }
                }
                main.post {
                    rememberDecision(pending, "approved", item.name, via, "证书")
                    _state.value = _state.value.copy(
                        selectedPending = null,
                        pending = _state.value.pending.filterNot { it.request_id == pending.request_id },
                        items = repo.items(),
                        lastApprovedItem = item.name,
                        lastApprovedRequester = pending.requester,
                        screen = Screen.Approved,
                        timedOut = false,
                        toast = "",
                    )
                    dropStaleNotifications()
                    releaseVault()
                }
            } catch (e: Exception) {
                main.post {
                    releaseVault()
                    _state.value = _state.value.copy(toast = e.message ?: "签发失败", message = e.message ?: "签发失败")
                }
            }
        }
    }

    fun denySelected() {
        val pending = _state.value.selectedPending ?: return
        val broker = client() ?: return
        bg {
            broker.decide(pending.request_id, "deny", null)
            main.post {
                rememberDecision(pending, "denied")
                _state.value = _state.value.copy(
                    selectedPending = null,
                    pending = _state.value.pending.filterNot { it.request_id == pending.request_id },
                    screen = Screen.Vault,
                    toast = "已拒绝",
                )
                dropStaleNotifications()
            }
        }
    }

    override fun onCleared() {
        // 待导入的明文密码随 ViewModel 一起丢掉。
        pendingImport = emptyList()
        super.onCleared()
    }
}
