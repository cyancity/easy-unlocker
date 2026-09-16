package io.github.cyancity.easyunlocker.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import io.github.cyancity.easyunlocker.Screen
import io.github.cyancity.easyunlocker.data.PairedDevice
import io.github.cyancity.easyunlocker.UiState
import kotlinx.coroutines.delay

private sealed class Sheet {
    data object None : Sheet()
    data object Picker : Sheet()
    data object AutoClose : Sheet()
    data object Password : Sheet()
    data object Export : Sheet()
    data class Menu(val id: String) : Sheet()
    data class Confirm(val id: String, val name: String) : Sheet()
    data class GatewayMenu(val id: String) : Sheet()
    data class GatewayRename(val id: String, val name: String) : Sheet()
    data class GatewayConfirm(val id: String, val name: String) : Sheet()
    data class DeviceRename(val device: PairedDevice) : Sheet()
}

@Composable
fun AppScreen(
    state: UiState,
    skipUnlockVisual: Boolean,
    onCreate: () -> Unit,
    onImportVault: () -> Unit,
    onUnlock: (String) -> Unit,
    onFingerprint: () -> Unit,
    onGo: (Screen) -> Unit,
    onOpenItem: (String) -> Unit,
    onStartEdit: (String?) -> Unit,
    onSaveItem: (String, String, String) -> Unit,
    onClearFormError: () -> Unit,
    onDelete: (String) -> Unit,
    onCopy: (String) -> Unit,
    onPair: (String, String, String) -> Unit,
    onSwitchGateway: (String) -> Unit,
    onRenameGateway: (String, String) -> Unit,
    onRemoveGateway: (String) -> Unit,
    onRefresh: () -> Unit,
    onSelectItem: (String) -> Unit,
    onSelectField: (String) -> Unit,
    onCreatePairCode: () -> Unit,
    onClearPairCode: () -> Unit,
    onRenewDevice: () -> Unit,
    onRenameDevice: (String, String) -> Unit,
    onRevokeDevice: (PairedDevice) -> Unit,
    onToast: (String) -> Unit,
    onOpenDevices: () -> Unit,
    onOpenHistory: (String) -> Unit,
    onRemember: (Boolean) -> Unit,
    onApprove: () -> Unit,
    onApprovePassword: (String) -> Unit,
    onDeny: () -> Unit,
    onRetry: () -> Unit,
    onExpire: (String) -> Unit,
    onClearIncoming: () -> Unit,
    onExport: () -> Unit,
    onExportSave: () -> Unit,
    onImport: () -> Unit,
    onConfirmVaultImport: (String) -> Unit,
    onCancelVaultImport: () -> Unit,
    onToggleImport: (Int) -> Unit,
    onToggleAllImport: (List<Int>) -> Unit,
    onConfirmImport: () -> Unit,
    onCancelImport: () -> Unit,
    onHaptics: (Boolean) -> Unit,
    onSound: (Boolean) -> Unit,
    onAutoClose: (Int) -> Unit,
    onUnlockPassword: (String) -> Unit,
    onSetPassword: (String) -> Unit,
    onClearPassword: () -> Unit,
    onToastShown: () -> Unit,
    onApprovedClose: () -> Unit,
) {
    var sheet by remember { mutableStateOf<Sheet>(Sheet.None) }
    // 条目列表的滚动位置提到这里：进详情/编辑会让 VaultPane 离开组合，放在它内部每次都会归零。
    val vaultScroll = rememberSaveable(saver = ScrollState.Saver) { ScrollState(0) }
    val tabbed = state.screen == Screen.Vault || state.screen == Screen.Pending || state.screen == Screen.Settings
    val openItem = state.items.firstOrNull { it.id == state.openItemId }
    val editItem = state.items.firstOrNull { it.id == state.editingId }

    BackHandler(enabled = state.screen == Screen.Item || state.screen == Screen.Edit || state.screen == Screen.Pair || state.screen == Screen.Pairings || state.screen == Screen.Approved || state.screen == Screen.History || state.screen == Screen.HistoryDetail || state.screen == Screen.Import || state.screen == Screen.Devices || sheet != Sheet.None) {
        when {
            sheet != Sheet.None -> sheet = Sheet.None
            state.screen == Screen.Import -> onCancelImport()
            state.screen == Screen.Devices -> onGo(Screen.Settings)
            state.screen == Screen.Item || state.screen == Screen.Approved -> onGo(Screen.Vault)
            state.screen == Screen.Edit -> onGo(if (state.editingId != null) Screen.Item else Screen.Vault)
            state.screen == Screen.Pair -> onGo(Screen.Pairings)
            state.screen == Screen.HistoryDetail -> onGo(Screen.History)
            state.screen == Screen.Pairings || state.screen == Screen.History -> onGo(Screen.Settings)
            else -> {}
        }
    }

    LaunchedEffect(state.toast) {
        if (state.toast.isNotBlank()) {
            delay(1800)
            onToastShown()
        }
    }
    LaunchedEffect(state.screen, state.autoCloseSeconds) {
        if (state.screen == Screen.Approved && state.autoCloseSeconds > 0) {
            delay(state.autoCloseSeconds * 1000L)
            onApprovedClose()
        }
    }

    Box(Modifier.fillMaxSize().background(Tokens.bg).statusBarsPadding().navigationBarsPadding().imePadding()) {
        Column(
            Modifier
                .fillMaxSize()
                .padding(bottom = if (tabbed) Dimens.navClearance else 0.dp),
        ) {
            when (state.screen) {
                Screen.Setup -> SetupPane(state.recoveryPreview, onCreate, onImportVault)
                Screen.Unlock -> UnlockPane(state, skipUnlockVisual, onFingerprint, onUnlock, onUnlockPassword)
                Screen.Vault -> VaultPane(state, vaultScroll, onOpenItem) { onStartEdit(null) }
                Screen.Item -> {
                    if (openItem != null) {
                        ItemPane(
                            item = openItem,
                            onBack = { onGo(Screen.Vault) },
                            onMenu = { sheet = Sheet.Menu(openItem.id) },
                            onCopy = onCopy,
                        )
                    } else {
                        VaultPane(state, vaultScroll, onOpenItem) { onStartEdit(null) }
                    }
                }
                Screen.Edit -> EditPane(
                    isNew = state.editingId == null,
                    initialName = editItem?.name.orEmpty(),
                    initialSecret = editItem?.secret.orEmpty(),
                    initialNote = editItem?.note.orEmpty(),
                    error = state.formError,
                    onChange = onClearFormError,
                    onCancel = { onGo(if (state.editingId != null) Screen.Item else Screen.Vault) },
                    onSave = onSaveItem,
                )
                Screen.Pending -> PendingPane(
                    state = state,
                    onChange = { sheet = Sheet.Picker },
                    onSelectField = onSelectField,
                    onRemember = onRemember,
                    onApprove = onApprove,
                    onApprovePassword = onApprovePassword,
                    onDeny = onDeny,
                    onRetry = onRetry,
                    onExpire = onExpire,
                    onArrived = onClearIncoming,
                )
                Screen.Approved -> ApprovedPane(state.lastApprovedItem, state.lastApprovedRequester) { onGo(Screen.Vault) }
                Screen.Devices -> DevicesPane(
                    state = state,
                    onBack = { onGo(Screen.Settings) },
                    onAdd = onCreatePairCode,
                    onRenew = onRenewDevice,
                    onRename = { sheet = Sheet.DeviceRename(it) },
                    onRevoke = onRevokeDevice,
                    onDismissCode = onClearPairCode,
                    onToast = onToast,
                )
                Screen.Settings -> SettingsPane(
                    state = state,
                    onGateways = { onGo(Screen.Pairings) },
                    onDevices = onOpenDevices,
                    onHistory = { onGo(Screen.History) },
                    onExport = { sheet = Sheet.Export },
                    onImport = onImport,
                    onImportVault = onImportVault,
                    onHaptics = onHaptics,
                    onSound = onSound,
                    onAutoClose = { sheet = Sheet.AutoClose },
                    onPassword = { sheet = Sheet.Password },
                )
                Screen.Import -> ImportPane(
                    state = state,
                    onToggle = onToggleImport,
                    onToggleAll = onToggleAllImport,
                    onConfirm = onConfirmImport,
                    onCancel = onCancelImport,
                )
                Screen.Pair -> PairPane({ onGo(Screen.Pairings) }, onPair, state.formError)
                Screen.Pairings -> PairingsPane(
                    state = state,
                    onAdd = { onGo(Screen.Pair) },
                    onSwitch = onSwitchGateway,
                    onMenu = { id -> sheet = Sheet.GatewayMenu(id) },
                    onBack = { onGo(Screen.Settings) },
                )
                Screen.History -> HistoryPane(state.history, onOpen = onOpenHistory) { onGo(Screen.Settings) }
                Screen.HistoryDetail -> {
                    val entry = state.history.firstOrNull { it.requestId == state.openHistoryId }
                    if (entry != null) {
                        HistoryDetailPane(entry) { onGo(Screen.History) }
                    } else {
                        HistoryPane(state.history, onOpen = onOpenHistory) { onGo(Screen.Settings) }
                    }
                }
            }
        }
        if (tabbed) {
            Box(
                Modifier
                    .align(Alignment.BottomCenter)
                    .padding(bottom = 18.dp),
            ) {
                PillNav(
                    active = when (state.screen) {
                        Screen.Pending -> Screen.Pending
                        Screen.Settings -> Screen.Settings
                        else -> Screen.Vault
                    },
                    pendingCount = state.pending.size,
                    onTab = { tab ->
                        if (tab == Screen.Pending) onRefresh() else onGo(tab)
                    },
                )
            }
        }
        if (state.toast.isNotBlank()) {
            Box(
                Modifier
                    .align(Alignment.BottomCenter)
                    .padding(bottom = if (tabbed) 96.dp else 32.dp),
            ) {
                ToastChip(state.toast)
            }
        }
        if (sheet != Sheet.None) {
            Box(Modifier.fillMaxSize()) {
                SheetScrim(onDismiss = { sheet = Sheet.None }) {
                    when (val s = sheet) {
                        Sheet.Picker -> PickerSheet(state.items) { id ->
                            onSelectItem(id)
                            sheet = Sheet.None
                        }
                        is Sheet.Menu -> ItemMenuSheet(
                            onEdit = {
                                sheet = Sheet.None
                                onStartEdit(s.id)
                            },
                            onDelete = {
                                val item = state.items.firstOrNull { it.id == s.id }
                                sheet = if (item != null) Sheet.Confirm(item.id, item.name) else Sheet.None
                            },
                        )
                        Sheet.AutoClose -> AutoCloseSheet(state.autoCloseSeconds) { sec ->
                            onAutoClose(sec)
                            sheet = Sheet.None
                        }
                        Sheet.Export -> ExportSheet(
                            onSave = {
                                sheet = Sheet.None
                                onExportSave()
                            },
                            onShare = {
                                sheet = Sheet.None
                                onExport()
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        Sheet.Password -> PasswordSheet(
                            hasPassword = state.hasPassword,
                            onSet = {
                                onSetPassword(it)
                                sheet = Sheet.None
                            },
                            onClear = {
                                onClearPassword()
                                sheet = Sheet.None
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        is Sheet.Confirm -> ConfirmDeleteSheet(
                            name = s.name,
                            onConfirm = {
                                onDelete(s.id)
                                sheet = Sheet.None
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        is Sheet.GatewayMenu -> GatewayMenuSheet(
                            onRename = {
                                val pairing = state.pairings.firstOrNull { it.id == s.id }
                                sheet = if (pairing != null) Sheet.GatewayRename(pairing.id, pairing.name) else Sheet.None
                            },
                            onDelete = {
                                val pairing = state.pairings.firstOrNull { it.id == s.id }
                                sheet = if (pairing != null) Sheet.GatewayConfirm(pairing.id, pairing.name) else Sheet.None
                            },
                        )
                        is Sheet.GatewayRename -> RenameGatewaySheet(
                            current = s.name,
                            onConfirm = { name ->
                                onRenameGateway(s.id, name)
                                sheet = Sheet.None
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        is Sheet.DeviceRename -> RenameDeviceSheet(
                            current = s.device.name,
                            onConfirm = { name ->
                                onRenameDevice(s.device.id, name)
                                sheet = Sheet.None
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        is Sheet.GatewayConfirm -> ConfirmRemoveGatewaySheet(
                            name = s.name,
                            onConfirm = {
                                onRemoveGateway(s.id)
                                sheet = Sheet.None
                            },
                            onCancel = { sheet = Sheet.None },
                        )
                        Sheet.None -> {}
                    }
                }
            }
        }
        if (state.vaultImportPrompt) {
            Box(Modifier.fillMaxSize()) {
                SheetScrim(onDismiss = onCancelVaultImport) {
                    ImportVaultSheet(
                        overwriting = state.screen != Screen.Setup,
                        onConfirm = onConfirmVaultImport,
                        onCancel = onCancelVaultImport,
                    )
                }
            }
        }
    }
}
