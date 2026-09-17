package io.github.cyancity.easyunlocker

import android.Manifest
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.content.pm.PackageManager
import android.media.AudioManager
import android.media.ToneGenerator
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.provider.Settings
import android.view.WindowManager
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import io.github.cyancity.easyunlocker.data.BitwardenAdapter
import io.github.cyancity.easyunlocker.data.VaultField
import io.github.cyancity.easyunlocker.ui.AppScreen
import io.github.cyancity.easyunlocker.ui.UnlockerTheme
import java.io.ByteArrayOutputStream
import java.io.File

class MainActivity : FragmentActivity() {
    private val vm: AppViewModel by viewModels()
    private var authenticating = false

    /** 文件选择/保存会让 Activity 走一次 onStop，期间不能锁库，否则拿回结果时库已上锁。 */
    private var picking = false
    private var skipUnlockVisual by mutableStateOf(false)
    private var tone: ToneGenerator? = null
    private var clipGen = 0
    private val main = Handler(Looper.getMainLooper())
    private val askNotify = registerForActivityResult(ActivityResultContracts.RequestPermission()) { _ -> }
    private val pickImport = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        picking = false
        if (uri == null) return@registerForActivityResult
        runCatching { readImportText(uri) }
            .onSuccess { vm.beginImport(it) }
            .onFailure { vm.toast(it.message ?: "读取文件失败") }
    }
    private val pickVault = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        picking = false
        if (uri == null) return@registerForActivityResult
        runCatching { readBytes(uri, MAX_VAULT_BYTES) }
            .onSuccess { vm.onVaultBackupPicked(it) }
            .onFailure { vm.toast(it.message ?: "读取文件失败") }
    }
    private val saveExport = registerForActivityResult(ActivityResultContracts.CreateDocument("application/octet-stream")) { uri ->
        picking = false
        if (uri == null) return@registerForActivityResult
        runCatching {
            contentResolver.openOutputStream(uri)?.use { it.write(vm.exportVault()) } ?: error("打不开这个位置")
        }.onFailure { vm.toast(it.message ?: "保存失败") }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.auto(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.auto(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT),
        )
        requestNotifyPermission()
        Notifications.ensureChannel(this)
        if (applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE == 0) {
            window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        }
        lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) {
                vm.startWatching()
                Push.fetch(this@MainActivity) { vm.registerPushToken(it) }
            }
            override fun onStop(owner: LifecycleOwner) {
                vm.stopWatching()
                if (!authenticating && !picking) vm.lock()
            }
        })
        handleDeepLink(intent)
        Push.fetch(this) { vm.registerPushToken(it) }
        setContent {
            val dark = isSystemInDarkTheme()
            val reduce = Settings.Global.getFloat(contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
            SideEffect {
                enableEdgeToEdge(
                    statusBarStyle = if (dark) {
                        SystemBarStyle.dark(android.graphics.Color.TRANSPARENT)
                    } else {
                        SystemBarStyle.light(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT)
                    },
                    navigationBarStyle = if (dark) {
                        SystemBarStyle.dark(android.graphics.Color.TRANSPARENT)
                    } else {
                        SystemBarStyle.light(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT)
                    },
                )
            }
            UnlockerTheme(darkTheme = dark, reduceMotion = reduce) {
                val state by vm.state.collectAsState()
                LaunchedEffect(state.incoming) {
                    if (state.incoming) buzzIncoming()
                }
                AppScreen(
                    state = state,
                    skipUnlockVisual = skipUnlockVisual && state.fromNotification && state.canFingerprint,
                    onCreate = { confirmBiometric("创建保险库") { vm.createVault() } },
                    onImportVault = {
                        picking = true
                        pickVault.launch(arrayOf("*/*"))
                    },
                    onUnlock = { code -> vm.unlock(code) },
                    onFingerprint = { confirmBiometric("指纹解锁") { vm.unlockWithFingerprint() } },
                    onUnlockPassword = { pw -> vm.unlockWithPassword(pw) },
                    onGo = vm::go,
                    onOpenItem = vm::openItem,
                    onStartEdit = vm::startEdit,
                    onSaveItem = vm::saveItem,
                    onClearFormError = vm::clearFormError,
                    onDelete = vm::deleteItem,
                    onCopy = { field -> copySecret(state, field) },
                    onPair = vm::pair,
                    onSwitchGateway = { id -> vm.switchPairing(id) },
                    onSwitchToPending = vm::switchPairingForPending,
                    onRenameGateway = vm::renamePairing,
                    onRenameDevice = vm::renameDevice,
                    onRemoveGateway = vm::removePairing,
                    onRefresh = { vm.go(Screen.Pending) },
                    onSelectItem = vm::selectItem,
                    onSelectField = vm::selectField,
                    onCreatePairCode = vm::createPairCode,
                    onClearPairCode = vm::clearPairCode,
                    onRenewDevice = vm::renewDevice,
                    onRevokeDevice = { pairingId, device -> vm.revokeDevice(pairingId, device.id) },
                    onToast = vm::toast,
                    onOpenDevices = vm::openDevices,
                    onOpenHistory = vm::openHistory,
                    onRemember = vm::setRememberAlias,
                    onApprove = { confirmBiometric("批准这次请求") { vm.approveSelected() } },
                    onApprovePassword = { pw -> vm.approveWithPassword(pw) },
                    onDeny = vm::denySelected,
                    onRetry = vm::retryPending,
                    onExpire = vm::expireRequest,
                    onClearIncoming = vm::clearIncoming,
                    onExport = { exportBackup() },
                    onExportSave = {
                        picking = true
                        saveExport.launch("easy-unlocker-backup.eu1")
                    },
                    onImport = {
                        picking = true
                        // 很多文件提供方把 .json 报成 octet-stream，所以给宽 MIME。
                        pickImport.launch(arrayOf("application/json", "text/plain", "*/*"))
                    },
                    onConfirmVaultImport = vm::confirmVaultImport,
                    onCancelVaultImport = vm::cancelVaultImport,
                    onToggleImport = vm::toggleImport,
                    onToggleAllImport = vm::toggleAllImport,
                    onConfirmImport = vm::confirmImport,
                    onCancelImport = vm::cancelImport,
                    onHaptics = vm::setHaptics,
                    onSound = { next ->
                        vm.setSound(next)
                        if (next) chime()
                    },
                    onAutoClose = vm::setAutoCloseSeconds,
                    onSetPassword = { pw -> vm.setPassword(pw) },
                    onClearPassword = { vm.clearPassword() },
                    onToastShown = vm::clearToast,
                    onApprovedClose = {
                        if (vm.state.value.screen == Screen.Approved) finish()
                    },
                )
                LaunchedEffect(state.screen) {
                    if (state.screen == Screen.Approved) {
                        buzzApproved()
                        chime()
                    }
                }
                // 请求批过 / 过期 / 撤销后，把通知栏里那条也收掉
                LaunchedEffect(state.clearNotifications) {
                    if (state.clearNotifications > 0) Notifications.clearAll(this@MainActivity)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleDeepLink(intent)
    }

    override fun onDestroy() {
        tone?.release()
        tone = null
        super.onDestroy()
    }

    private fun requestNotifyPermission() {
        if (Build.VERSION.SDK_INT < 33) return
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) return
        askNotify.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    private fun handleDeepLink(intent: Intent?) {
        if (intent == null) return
        val fromPush = intent.data?.scheme == "easyunlocker" ||
            !intent.getStringExtra("request_id").isNullOrBlank() ||
            intent.action == "OPEN_PENDING"
        if (fromPush) {
            skipUnlockVisual = true
            // 推送里带 gateway（哪台网关发的）时先切过去；旧 Broker 不带，行为不变
            vm.markFromNotification(intent.getStringExtra("gateway"))
        }
    }

    private fun vibrator(): Vibrator {
        return if (Build.VERSION.SDK_INT >= 31) {
            (getSystemService(VIBRATOR_MANAGER_SERVICE) as VibratorManager).defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            getSystemService(VIBRATOR_SERVICE) as Vibrator
        }
    }

    private fun buzzIncoming() {
        if (!vm.state.value.haptics) return
        vibrator().vibrate(VibrationEffect.createOneShot(40, VibrationEffect.DEFAULT_AMPLITUDE))
    }

    private fun buzzApproved() {
        if (!vm.state.value.haptics) return
        vibrator().vibrate(
            VibrationEffect.createWaveform(longArrayOf(0, 24, 70, 46), -1),
        )
    }

    private fun chime() {
        if (!vm.state.value.sound) return
        val am = getSystemService(AUDIO_SERVICE) as AudioManager
        if (am.ringerMode != AudioManager.RINGER_MODE_NORMAL) return
        runCatching {
            val tg = tone ?: ToneGenerator(AudioManager.STREAM_NOTIFICATION, 60).also { tone = it }
            tg.startTone(ToneGenerator.TONE_PROP_ACK, 180)
        }
    }

    private fun copySecret(state: io.github.cyancity.easyunlocker.UiState, field: String) {
        val item = state.items.firstOrNull { it.id == state.openItemId } ?: return
        val value = if (field == VaultField.NOTE) item.note else item.secret
        if (value.isEmpty()) {
            vm.toast(if (field == VaultField.NOTE) "这条没有备注" else "这条没有密码")
            return
        }
        val clip = getSystemService(CLIPBOARD_SERVICE) as ClipboardManager
        clip.setPrimaryClip(ClipData.newPlainText("easy-unlocker", value))
        val gen = ++clipGen
        main.postDelayed({
            if (gen == clipGen) clip.clearPrimaryClip()
        }, 30_000)
        vm.toast("已复制 · 30 秒后清空剪贴板")
    }

    /** 读进内存就直接解析，不落盘、不留临时文件；带大小上限，避免整库导出把内存撑爆。 */
    private fun readImportText(uri: Uri): String =
        readBytes(uri, BitwardenAdapter.MAX_BYTES, "文件太大了，先在 Bitwarden 里导出成 JSON。")
            .toString(Charsets.UTF_8)

    private fun readBytes(uri: Uri, max: Int, tooBig: String = "文件太大了"): ByteArray {
        val stream = contentResolver.openInputStream(uri) ?: error("打不开这个文件")
        stream.use {
            val out = ByteArrayOutputStream()
            val buf = ByteArray(64 * 1024)
            var total = 0
            while (true) {
                val read = it.read(buf)
                if (read < 0) break
                total += read
                if (total > max) error(tooBig)
                out.write(buf, 0, read)
            }
            return out.toByteArray()
        }
    }

    private fun exportBackup() {
        runCatching {
            val file = File(cacheDir, "easy-unlocker-backup.eu1")
            file.writeBytes(vm.exportVault())
            val uri = FileProvider.getUriForFile(this, "$packageName.files", file)
            startActivity(Intent(Intent.ACTION_SEND).apply {
                type = "application/octet-stream"
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            })
        }
    }

    private fun confirmBiometric(title: String, onOk: () -> Unit) {
        val manager = BiometricManager.from(this)
        val can = manager.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
        if (can != BiometricManager.BIOMETRIC_SUCCESS) {
            onOk()
            return
        }
        authenticating = true
        val prompt = BiometricPrompt(this, ContextCompat.getMainExecutor(this), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                skipUnlockVisual = false
                onOk()
                authenticating = false
            }
            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                authenticating = false
                skipUnlockVisual = false
            }
        })
        prompt.authenticate(
            BiometricPrompt.PromptInfo.Builder()
                .setTitle(title)
                .setSubtitle("认证只在本机完成")
                .setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
                .build()
        )
    }

    companion object {
        private const val MAX_VAULT_BYTES = 64 * 1024 * 1024
    }
}
