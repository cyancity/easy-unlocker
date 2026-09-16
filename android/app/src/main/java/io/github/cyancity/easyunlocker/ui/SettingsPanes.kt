package io.github.cyancity.easyunlocker.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import io.github.cyancity.easyunlocker.UiState
import io.github.cyancity.easyunlocker.data.HistoryEntry
import io.github.cyancity.easyunlocker.data.ImportOption
import io.github.cyancity.easyunlocker.data.PairedDevice
import io.github.cyancity.easyunlocker.data.Pairing
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun SettingsPane(
    state: UiState,
    onGateways: () -> Unit,
    onDevices: () -> Unit,
    onHistory: () -> Unit,
    onExport: () -> Unit,
    onImport: () -> Unit,
    onImportVault: () -> Unit,
    onHaptics: (Boolean) -> Unit,
    onSound: (Boolean) -> Unit,
    onAutoClose: () -> Unit,
    onPassword: () -> Unit,
) {
    Column(Modifier.fillMaxSize()) {
        PageTitle("设置")
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState()),
        ) {
            HairList {
                SettingRow(
                    label = "网关",
                    value = when {
                        state.pairings.isEmpty() -> "未配对"
                        state.pairings.size > 1 -> "${state.deviceName} · 共 ${state.pairings.size} 个"
                        else -> state.deviceName.ifBlank { gatewayHost(state.brokerUrl) }
                    },
                    onClick = onGateways,
                )
                SettingRow(
                    label = "设备",
                    value = when {
                        state.devices.isEmpty() -> "配对码与设备管理"
                        else -> "${state.devices.size} 台已配对"
                    },
                    onClick = onDevices,
                )
                SettingRow(
                    label = "解锁密码",
                    value = if (state.hasPassword) "已设置" else "未设置",
                    onClick = onPassword,
                )
                SettingRow(
                    label = "批准记录",
                    value = if (state.history.isEmpty()) "" else "${state.history.size} 条",
                    onClick = onHistory,
                )
                SettingRow(
                    label = "批准后自动关闭",
                    value = if (state.autoCloseSeconds <= 0) "不关闭" else "${state.autoCloseSeconds} 秒",
                    onClick = onAutoClose,
                )
                SettingRow(
                    label = "导出密文备份",
                    value = if (state.lastExportAt.isBlank()) "" else "上次 ${state.lastExportAt}",
                    onClick = onExport,
                )
                SettingRow(
                    label = "导入密文备份",
                    value = "覆盖当前库 · 需要恢复码",
                    onClick = onImportVault,
                )
                SettingRow(
                    label = "导入 Bitwarden 备份",
                    value = "仅支持 JSON" + (if (state.lastImportAt.isBlank()) "" else " · 上次 ${state.lastImportAt}"),
                    onClick = onImport,
                )
                SettingRow(
                    label = "放行时震动",
                    onClick = { onHaptics(!state.haptics) },
                    trailing = { SwitchTrack(state.haptics) },
                )
                SettingRow(
                    label = "放行时提示音",
                    onClick = { onSound(!state.sound) },
                    trailing = { SwitchTrack(state.sound) },
                )
                SettingRow(label = "推送", value = "FCM", last = true)
            }
            Text(
                "恢复码 Documents/easy-unlocker-recovery.txt",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                modifier = Modifier.padding(bottom = 22.dp, top = 24.dp),
            )
        }
    }
}

/** 网关列表：一台网关一条，能加、能切、能删。切网关不动本地条目。 */
@Composable
fun PairingsPane(
    state: UiState,
    onAdd: () -> Unit,
    onSwitch: (String) -> Unit,
    onMenu: (String) -> Unit,
    onBack: () -> Unit,
) {
    Column(Modifier.fillMaxSize()) {
        AppBar("网关", onBack, trailing = { AddBtn(onAdd) })
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState()),
        ) {
            Text(
                "请求从哪台网关进来，就在那台上批准。切换网关只换「往哪儿发」，本地条目不受影响。",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(bottom = 16.dp),
            )
            if (state.pairings.isEmpty()) {
                Column(
                    Modifier.fillMaxWidth().padding(top = 36.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(14.dp),
                ) {
                    Icon(UnlockerIcons.Phone, contentDescription = null, tint = Tokens.dim, modifier = Modifier.size(30.dp))
                    Text("还没有网关", color = Tokens.fg, fontSize = 20.sp, fontWeight = W650, lineHeight = 28.sp)
                    Text(
                        "贴一台 Broker 的地址和 pairing token，手机才能收到批准请求。",
                        color = Tokens.muted,
                        fontSize = 12.sp,
                        lineHeight = 18.sp,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.padding(horizontal = 16.dp),
                    )
                    QuietButton("添加网关", modifier = Modifier.padding(top = 4.dp).fillMaxWidth(0.55f), onClick = onAdd)
                }
            } else {
                HairList {
                    state.pairings.forEachIndexed { i, pairing ->
                        GatewayRow(
                            pairing = pairing,
                            active = pairing.id == state.activePairingId,
                            last = i == state.pairings.lastIndex,
                            onSwitch = { onSwitch(pairing.id) },
                            onMenu = { onMenu(pairing.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun GatewayRow(
    pairing: Pairing,
    active: Boolean,
    last: Boolean,
    onSwitch: () -> Unit,
    onMenu: () -> Unit,
) {
    Row(
        Modifier.fillMaxWidth().height(Dimens.listRow).padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f).padding(end = 8.dp)) {
            Text(
                pairing.name,
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                gatewayHost(pairing.url),
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 3.dp),
            )
        }
        if (active) {
            Chip("当前", ok = true)
        } else {
            Box(
                Modifier
                    .height(36.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .border(1.dp, Tokens.borderStrong, RoundedCornerShape(999.dp))
                    .clickable(role = Role.Button, onClick = onSwitch)
                    .semantics { contentDescription = "切到此网关" }
                    .padding(horizontal = 13.dp)
                    .focusRing(18.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text("切换", color = Tokens.fg, fontSize = 13.sp, fontWeight = W650)
            }
        }
        Box(Modifier.size(6.dp))
        IconBtn(UnlockerIcons.More, "网关操作", onMenu)
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

@Composable
fun GatewayMenuSheet(onRename: () -> Unit, onDelete: () -> Unit) {
    Text("网关操作", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 12.dp))
    HairList {
        Box(
            Modifier
                .fillMaxWidth()
                .height(60.dp)
                .clickable(role = Role.Button, onClick = onRename)
                .padding(vertical = 12.dp),
            contentAlignment = Alignment.CenterStart,
        ) {
            Text("改个名字", color = Tokens.fg, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650)
        }
        Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
        Box(
            Modifier
                .fillMaxWidth()
                .height(60.dp)
                .clickable(role = Role.Button, onClick = onDelete)
                .padding(vertical = 12.dp),
            contentAlignment = Alignment.CenterStart,
        ) {
            Text("移除这台网关", color = Tokens.dangerText, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650)
        }
    }
}

@Composable
fun RenameGatewaySheet(current: String, onConfirm: (String) -> Unit, onCancel: () -> Unit) {
    var name by remember(current) { mutableStateOf(current) }
    Text("改个名字", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(horizontal = 6.dp))
    Text(
        "只改这台手机上的显示名；请求从哪台来、往哪台批都不受影响。",
        color = Tokens.muted,
        fontSize = 12.sp,
        lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 16.dp),
    )
    Field(name, { name = it }, "网关名", mono = false, placeholder = "my-vps / cf-worker")
    PrimaryButton("保存") { onConfirm(name) }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

@Composable
fun RenameDeviceSheet(current: String, onConfirm: (String) -> Unit, onCancel: () -> Unit) {
    var name by remember(current) { mutableStateOf(current) }
    Text("改个名字", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(horizontal = 6.dp))
    Text(
        "改的是 Broker 上的设备名，设备列表里所有人都看到新名字。",
        color = Tokens.muted,
        fontSize = 12.sp,
        lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 16.dp),
    )
    Field(name, { name = it }, "设备名", mono = false, placeholder = "my-laptop")
    PrimaryButton("保存") { onConfirm(name) }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

@Composable
fun ConfirmRemoveGatewaySheet(name: String, onConfirm: () -> Unit, onCancel: () -> Unit) {
    Text("移除 $name？", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(horizontal = 6.dp))
    Text(
        "只是不再往这台网关收/批请求；本地条目和另一台网关都不受影响。以后要用可以重新添加。",
        color = Tokens.muted,
        fontSize = 12.sp,
        lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 18.dp),
    )
    Box(
        Modifier
            .fillMaxWidth()
            .height(Dimens.mainButton)
            .clip(RoundedCornerShape(Dimens.rButton))
            .background(Tokens.danger)
            .clickable(role = Role.Button, onClick = onConfirm)
            .focusRing(Dimens.rButton),
        contentAlignment = Alignment.Center,
    ) {
        Text("移除", color = Tokens.onDanger, fontSize = 16.sp, fontWeight = W650)
    }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

/**
 * 解锁密码的设置/更换/清除。密码是指纹之外的兜底认证：解锁保险库、
 * 批准请求都能用。只存在这台手机上；重导或新建库后要重设。
 */
@Composable
fun PasswordSheet(hasPassword: Boolean, onSet: (String) -> Unit, onClear: () -> Unit, onCancel: () -> Unit) {
    var pw by remember { mutableStateOf("") }
    var confirm by remember { mutableStateOf("") }
    Text(
        if (hasPassword) "更换解锁密码" else "设置解锁密码",
        color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp,
        modifier = Modifier.padding(horizontal = 6.dp),
    )
    Text(
        "指纹不行的时候用它。密码只在这台手机上，至少 6 位；新建或重导入库后要重设。",
        color = Tokens.muted, fontSize = 12.sp, lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 16.dp),
    )
    Field(pw, { pw = it }, "解锁密码", secret = true)
    Field(confirm, { confirm = it }, "再输一次", secret = true)
    if (confirm.isNotEmpty() && pw != confirm) {
        Text(
            "两次输入不一样",
            color = Tokens.warnText, fontSize = 12.sp, lineHeight = 18.sp,
            modifier = Modifier.padding(start = 6.dp, bottom = 12.dp),
        )
    }
    PrimaryButton(
        if (hasPassword) "更新密码" else "设置密码",
        enabled = pw.length >= 6 && pw == confirm,
    ) { onSet(pw) }
    if (hasPassword) {
        Spacer(Modifier.height(10.dp))
        DeclineButton("清除密码", onClear)
    }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

/** 导出去哪儿：存文件走系统文件管理器，分享走系统分享页。产物是同一份密文。 */
@Composable
fun ExportSheet(onSave: () -> Unit, onShare: () -> Unit, onCancel: () -> Unit) {
    Text(
        "导出密文备份",
        color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp,
        modifier = Modifier.padding(horizontal = 6.dp),
    )
    Text(
        "拿到的是加密文件，换机时要配恢复码才解得开。",
        color = Tokens.muted, fontSize = 12.sp, lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 16.dp),
    )
    PrimaryButton("保存到文件", onClick = onSave)
    Spacer(Modifier.height(10.dp))
    QuietButton("分享…", onClick = onShare)
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

/**
 * 导入 .eu1 密文备份：整库覆盖，不是合并。备份里的恢复码是唯一能开它的钥匙；
 * 覆盖后原库的解锁密码作废（指纹 wrap 会在解锁时按新库重包）。
 */
@Composable
fun ImportVaultSheet(overwriting: Boolean, onConfirm: (String) -> Unit, onCancel: () -> Unit) {
    var code by remember { mutableStateOf("") }
    Text(
        "导入密文备份",
        color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp,
        modifier = Modifier.padding(horizontal = 6.dp),
    )
    Text(
        if (overwriting) "导入会整个换掉当前库，不是合并。输入这份备份的恢复码。"
        else "用备份文件恢复保险库。输入这份备份的恢复码。",
        color = if (overwriting) Tokens.warnText else Tokens.muted,
        fontSize = 12.sp, lineHeight = 18.sp,
        modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 16.dp),
    )
    Field(code, { code = it }, "恢复码", placeholder = "粘贴恢复码")
    PrimaryButton("导入", enabled = code.isNotBlank()) { onConfirm(code) }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}

/** 显示用：去掉协议头，空值给个占位。 */
fun gatewayHost(url: String): String =
    url.removePrefix("https://").removePrefix("http://").trimEnd('/').ifBlank { "未设置" }

@Composable
fun PairPane(onBack: () -> Unit, onPair: (String, String, String) -> Unit, error: String) {
    var url by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var name by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize()) {
        AppBar("添加网关", onBack, backLabel = "取消", backIcon = UnlockerIcons.Close)
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState())
                .padding(top = 14.dp),
        ) {
            Text(
                "一台 Broker = 一个网关。可以同时加多台，随时切换，互不影响。",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(bottom = 18.dp),
            )
            Field(name, { name = it }, "网关名", mono = false, placeholder = "my-vps / cf-worker")
            Field(url, { url = it }, "Broker", placeholder = "https://broker.example.com")
            Field(token, { token = it }, "pairing token")
            if (error.isNotBlank()) {
                Text(error, color = Tokens.dangerText, fontSize = 12.sp, fontWeight = W650, lineHeight = 18.sp)
            }
        }
        ActionsColumn {
            PrimaryButton("配对并添加") { onPair(url, token, name) }
        }
    }
}

@Composable
fun ImportPane(
    state: UiState,
    onToggle: (Int) -> Unit,
    onToggleAll: (List<Int>) -> Unit,
    onConfirm: () -> Unit,
    onCancel: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    val visible = remember(state.importOptions, query) {
        val q = query.trim().lowercase()
        if (q.isEmpty()) {
            state.importOptions
        } else {
            state.importOptions.filter {
                it.name.lowercase().contains(q) || it.username.lowercase().contains(q)
            }
        }
    }
    val visibleIds = visible.map { it.id }
    val allOn = visibleIds.isNotEmpty() && visibleIds.all { it in state.importSelected }
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            AppBar(
                title = "导入 Bitwarden",
                onBack = onCancel,
                backLabel = "取消",
                backIcon = UnlockerIcons.Close,
                trailing = {
                    if (visibleIds.isNotEmpty()) {
                        Box(
                            Modifier
                                .height(Dimens.touch)
                                .clip(RoundedCornerShape(999.dp))
                                .clickable(role = Role.Button, onClick = { onToggleAll(visibleIds) })
                                .padding(horizontal = 12.dp)
                                .focusRing(24.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            Text(
                                if (allOn) "取消全选" else "全选",
                                color = Tokens.accentText,
                                fontSize = 13.sp,
                                fontWeight = W650,
                            )
                        }
                    }
                },
            )
            Column(Modifier.weight(1f).fillMaxWidth().padding(horizontal = Dimens.page)) {
                Text(
                    state.importNotice,
                    color = Tokens.muted,
                    fontSize = 12.sp,
                    lineHeight = 18.sp,
                    modifier = Modifier.padding(bottom = 14.dp),
                )
                Field(query, { query = it }, "搜索", mono = false, placeholder = "条目名或用户名")
                // 底部留出悬浮胶囊的高度，最后一条能滚到按钮上方。
                Column(
                    Modifier
                        .weight(1f)
                        .fillMaxWidth()
                        .verticalScroll(rememberScrollState())
                        .padding(bottom = Dimens.navClearance),
                ) {
                    when {
                        state.importOptions.isEmpty() -> Text("没有可导入的条目。", color = Tokens.muted, fontSize = 13.sp)
                        visible.isEmpty() -> Text("没有匹配的条目。", color = Tokens.muted, fontSize = 13.sp)
                        else -> HairList {
                            visible.forEachIndexed { i, option ->
                                ImportRow(option, option.id in state.importSelected, i == visible.lastIndex) {
                                    onToggle(option.id)
                                }
                            }
                        }
                    }
                }
            }
        }
        Box(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .padding(start = Dimens.page, end = Dimens.page, bottom = 18.dp),
        ) {
            PrimaryButton(
                text = "导入 ${state.importSelected.size} 条",
                enabled = state.importSelected.isNotEmpty(),
                capsule = true,
                onClick = onConfirm,
            )
        }
    }
}

@Composable
private fun ImportRow(option: ImportOption, selected: Boolean, last: Boolean, onToggle: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .height(Dimens.listRow)
            .clickable(role = Role.Button, onClick = onToggle)
            .padding(vertical = 12.dp)
            .focusRing(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                option.name,
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (option.username.isNotBlank() && option.username.lowercase() != option.name.lowercase()) {
                Text(
                    option.username,
                    color = Tokens.muted,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
        if (selected) {
            Icon(UnlockerIcons.Check, contentDescription = null, tint = Tokens.accent, modifier = Modifier.size(18.dp))
        } else {
            Box(Modifier.size(18.dp).clip(CircleShape).border(1.dp, Tokens.borderStrong, CircleShape))
        }
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

@Composable
fun HistoryPane(entries: List<HistoryEntry>, onOpen: (String) -> Unit, onBack: () -> Unit) {
    var filter by remember { mutableStateOf("all") }
    val filtered = if (filter == "all") entries else entries.filter { it.decision == filter }
    Column(Modifier.fillMaxSize()) {
        AppBar("批准记录", onBack)
        Row(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .padding(bottom = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            HistoryFilterPill("全部", filter == "all") { filter = "all" }
            HistoryFilterPill("已放行", filter == "approved") { filter = "approved" }
            HistoryFilterPill("已拒绝", filter == "denied") { filter = "denied" }
            HistoryFilterPill("已超时", filter == "expired") { filter = "expired" }
        }
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState()),
        ) {
            if (entries.isEmpty()) {
                Column(
                    Modifier.fillMaxWidth().padding(top = 48.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(14.dp),
                ) {
                    Icon(UnlockerIcons.Bell, contentDescription = null, tint = Tokens.dim, modifier = Modifier.size(30.dp))
                    Text("还没有记录", color = Tokens.fg, fontSize = 20.sp, fontWeight = W650, lineHeight = 28.sp, letterSpacing = 0.sp)
                    Text(
                        "放行、拒绝和超时会留在这台手机上，不上传。",
                        color = Tokens.muted,
                        fontSize = 12.sp,
                        lineHeight = 18.sp,
                        textAlign = TextAlign.Center,
                    )
                }
            } else {
                if (filtered.isEmpty()) {
                    Text(
                        "这个状态下还没有记录。",
                        color = Tokens.muted,
                        fontSize = 12.sp,
                        modifier = Modifier.padding(top = 14.dp),
                    )
                } else {
                    HairList {
                        filtered.forEachIndexed { i, e ->
                            HistoryRow(e, last = i == filtered.lastIndex, onClick = { onOpen(e.requestId) })
                        }
                    }
                    Text(
                        "放行、拒绝和超时会留在这台手机上，不上传。",
                        color = Tokens.muted,
                        fontSize = 12.sp,
                        lineHeight = 18.sp,
                        modifier = Modifier.padding(top = 14.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun HistoryFilterPill(label: String, active: Boolean, onClick: () -> Unit) {
    val shape = RoundedCornerShape(999.dp)
    Box(
        Modifier
            .clip(shape)
            .background(if (active) Tokens.accentQuiet else Tokens.raised)
            .border(1.dp, if (active) Tokens.accent else Tokens.borderStrong, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 7.dp)
            .focusRing(999.dp),
    ) {
        Text(
            label,
            color = if (active) Tokens.accentText else Tokens.muted,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            fontWeight = W650,
        )
    }
}

@Composable
private fun HistoryRow(entry: HistoryEntry, last: Boolean, onClick: () -> Unit) {
    val whenText = SimpleDateFormat("M月d日 HH:mm", Locale.CHINA).format(Date(entry.at))
    val (label, ok, warn) = when (entry.decision) {
        "approved" -> Triple("已放行", true, false)
        "denied" -> Triple("已拒绝", false, false)
        else -> Triple("已超时", false, true)
    }
    val who = entry.requester.ifBlank { "未知" }
    Row(
        Modifier
            .fillMaxWidth()
            .height(Dimens.listRow)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(vertical = 12.dp)
            .focusRing(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                entry.item,
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                letterSpacing = (-0.32).sp,
            )
            Text(
                "$who · $whenText",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                modifier = Modifier.padding(top = 4.dp),
            )
        }
        Chip(label, warn = warn, ok = ok)
        Icon(
            UnlockerIcons.Chevron,
            contentDescription = null,
            tint = Tokens.dim,
            modifier = Modifier.padding(start = 6.dp).size(15.dp),
        )
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

/** 记录详情（对齐 iOS HistoryDetailView）：请求快照卡 + 处理结果分组列表。 */
@Composable
fun HistoryDetailPane(entry: HistoryEntry, onBack: () -> Unit) {
    val whenText = SimpleDateFormat("M月d日 HH:mm", Locale.CHINA).format(Date(entry.at))
    Column(Modifier.fillMaxSize()) {
        AppBar("记录详情", onBack)
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState())
                .padding(top = 10.dp),
        ) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(Dimens.rCard))
                    .background(Tokens.surface)
                    .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
                    .padding(16.dp),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        Modifier
                            .size(44.dp)
                            .clip(RoundedCornerShape(12.dp))
                            .background(Tokens.raised),
                        contentAlignment = Alignment.Center,
                    ) {
                        Icon(UnlockerIcons.Terminal, contentDescription = null, tint = Tokens.muted, modifier = Modifier.size(21.dp))
                    }
                    Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                        Text(
                            entry.requester.ifBlank { "未知" },
                            color = Tokens.fg,
                            fontSize = 17.sp,
                            fontWeight = FontWeight.SemiBold,
                        )
                        Text("$whenText 发起", color = Tokens.muted, fontSize = 14.sp, modifier = Modifier.padding(top = 1.dp))
                    }
                    DecisionChip(entry.decision)
                }
                Text(
                    entry.item,
                    color = Tokens.fg,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 24.sp,
                    fontWeight = FontWeight.SemiBold,
                    letterSpacing = (-0.7).sp,
                    modifier = Modifier.padding(top = 18.dp),
                )
                if (entry.purpose.isNotBlank()) {
                    HistoryFieldLabel("用途", Modifier.padding(top = 14.dp))
                    Text(
                        entry.purpose,
                        color = Tokens.fg,
                        fontSize = 14.sp,
                        lineHeight = 21.sp,
                        modifier = Modifier.padding(top = 5.dp),
                    )
                }
                if (entry.commandLine.isNotBlank()) {
                    HistoryFieldLabel("命令", Modifier.padding(top = 14.dp))
                    Box(Modifier.padding(top = 5.dp)) {
                        CommandBlock(entry.commandLine)
                    }
                }
            }
            Text(
                "处理结果",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                letterSpacing = 1.1.sp,
                modifier = Modifier.padding(top = 22.dp, bottom = 8.dp),
            )
            HairList {
                HistoryDetailRow("结果", decisionText(entry.decision), decisionColor(entry.decision))
                HistoryDetailRow("批准方式", viaText(entry.via))
                HistoryDetailRow("交付字段", entry.field.ifBlank { "—" })
                HistoryDetailRow("耗时", tookText(entry), last = true)
            }
            Text(
                "这是当时那条请求的完整快照，值本身不留痕。",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(top = 14.dp),
            )
        }
    }
}

@Composable
private fun HistoryFieldLabel(label: String, modifier: Modifier = Modifier) {
    Text(
        label,
        color = Tokens.muted,
        fontFamily = FontFamily.Monospace,
        fontSize = 11.sp,
        letterSpacing = 1.1.sp,
        modifier = modifier,
    )
}

@Composable
private fun HistoryDetailRow(label: String, value: String, color: Color = Tokens.muted, last: Boolean = false) {
    Row(
        Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = Tokens.fg, fontSize = 15.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.weight(1f))
        Text(value, color = color, fontSize = 14.sp, maxLines = 1)
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

@Composable
private fun DecisionChip(decision: String) {
    when (decision) {
        "approved" -> Chip("已放行", ok = true)
        "denied" -> Chip("已拒绝")
        else -> Chip("已超时", warn = true)
    }
}

private fun decisionText(decision: String): String = when (decision) {
    "approved" -> "已放行"
    "denied" -> "已拒绝"
    else -> "已超时"
}

@Composable
private fun decisionColor(decision: String): Color = when (decision) {
    "approved" -> Tokens.okText
    "denied" -> Tokens.dangerText
    else -> Tokens.warnText
}

private fun viaText(via: String): String = when (via) {
    "faceid" -> "Face ID"
    "touchid" -> "Touch ID"
    "opticid" -> "Optic ID"
    "biometric" -> "生物识别"
    "password" -> "密码"
    "" -> "—"
    else -> via
}

private fun tookText(entry: HistoryEntry): String {
    if (entry.tookMs <= 0) return "—"
    val s = entry.tookMs / 1000
    return when (entry.decision) {
        "expired" -> "$s 秒无响应"
        "denied" -> "$s 秒后拒绝"
        else -> "$s 秒"
    }
}

@Composable
fun AutoCloseSheet(current: Int, onPick: (Int) -> Unit) {
    Text("批准后自动关闭", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 12.dp))
    val options = listOf(0 to "不关闭", 3 to "3 秒", 5 to "5 秒", 10 to "10 秒", 30 to "30 秒")
    HairList {
        options.forEachIndexed { i, (sec, label) ->
            val on = current == sec
            Row(
                Modifier
                    .fillMaxWidth()
                    .height(56.dp)
                    .clickable(role = Role.Button, onClick = { onPick(sec) })
                    .padding(vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    label,
                    color = Tokens.fg,
                    fontSize = 16.sp,
                    fontWeight = if (on) FontWeight.SemiBold else FontWeight.Normal,
                    modifier = Modifier.weight(1f),
                )
                if (on) Text("当前", color = Tokens.okText, fontFamily = FontFamily.Monospace, fontSize = 12.sp)
            }
            if (i != options.lastIndex) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
        }
    }
}

// ---------- 设备管理：本机（审批端）与外部设备（请求端）分开 ----------

private fun isoDate(iso: String): String =
    runCatching {
        java.time.Instant.parse(iso).atZone(java.time.ZoneId.systemDefault()).toLocalDate().toString()
    }.getOrDefault(iso.take(10))

private fun deviceMeta(device: PairedDevice): String {
    val used = if (device.lastUsedAt.isBlank()) "从未使用" else "最后使用 ${isoDate(device.lastUsedAt)}"
    return "$used · 有效期至 ${isoDate(device.expiresAt)}"
}

@Composable
private fun SelfDeviceCard(
    device: PairedDevice,
    logoutArmed: Boolean,
    onRenew: () -> Unit,
    onRename: () -> Unit,
    onLogout: () -> Unit,
) {
    Box(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(Dimens.rCard))
            .background(Tokens.surface)
            .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
            .padding(horizontal = 16.dp, vertical = 14.dp),
    ) {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(device.name.ifBlank { "这台手机" }, color = Tokens.fg, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650, modifier = Modifier.weight(1f))
                Chip("审批端", ok = true)
            }
            Text(deviceMeta(device), color = Tokens.muted, fontSize = 11.sp, lineHeight = 16.sp, modifier = Modifier.padding(top = 6.dp))
            Row(
                Modifier.padding(top = 12.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    Modifier
                        .clip(RoundedCornerShape(999.dp))
                        .background(Tokens.raised)
                        .border(1.dp, Tokens.borderStrong, RoundedCornerShape(999.dp))
                        .clickable(role = Role.Button, onClick = onRenew)
                        .padding(horizontal = 12.dp, vertical = 6.dp)
                        .focusRing(999.dp),
                ) {
                    Text("续期 180 天", color = Tokens.accentText, fontFamily = FontFamily.Monospace, fontSize = 11.sp, fontWeight = W650)
                }
                Box(
                    Modifier
                        .clip(RoundedCornerShape(999.dp))
                        .background(Tokens.raised)
                        .border(1.dp, Tokens.borderStrong, RoundedCornerShape(999.dp))
                        .clickable(role = Role.Button, onClick = onRename)
                        .padding(horizontal = 12.dp, vertical = 6.dp)
                        .focusRing(999.dp),
                ) {
                    Text("改名", color = Tokens.accentText, fontFamily = FontFamily.Monospace, fontSize = 11.sp, fontWeight = W650)
                }
                Box(
                    Modifier
                        .clip(RoundedCornerShape(999.dp))
                        .background(if (logoutArmed) Tokens.danger else Tokens.raised)
                        .border(1.dp, if (logoutArmed) Tokens.danger else Tokens.borderStrong, RoundedCornerShape(999.dp))
                        .clickable(role = Role.Button, onClick = onLogout)
                        .padding(horizontal = 12.dp, vertical = 6.dp)
                        .focusRing(999.dp),
                ) {
                    Text(
                        if (logoutArmed) "再点一次确认登出" else "登出这台",
                        color = if (logoutArmed) Tokens.onDanger else Tokens.dangerText,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 11.sp,
                        fontWeight = W650,
                    )
                }
            }
        }
    }
}

@Composable
private fun ExternalDeviceRow(device: PairedDevice, onRename: (PairedDevice) -> Unit, onRevoke: (PairedDevice) -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Column(Modifier.weight(1f)) {
            Text(device.name.ifBlank { "未命名设备" }, color = Tokens.fg, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650)
            Text(deviceMeta(device), color = Tokens.muted, fontSize = 11.sp, lineHeight = 16.sp)
        }
        Text(
            "改名",
            color = Tokens.accentText,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            fontWeight = W650,
            modifier = Modifier
                .clip(RoundedCornerShape(999.dp))
                .clickable(role = Role.Button) { onRename(device) }
                .padding(horizontal = 12.dp, vertical = 6.dp),
        )
        Text(
            "撤销",
            color = Tokens.dangerText,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            fontWeight = W650,
            modifier = Modifier
                .clip(RoundedCornerShape(999.dp))
                .clickable(role = Role.Button) { onRevoke(device) }
                .padding(horizontal = 12.dp, vertical = 6.dp),
        )
    }
}

@Composable
fun DevicesPane(
    state: UiState,
    onBack: () -> Unit,
    onAdd: () -> Unit,
    onRenew: () -> Unit,
    onRename: (PairedDevice) -> Unit,
    onRevoke: (PairedDevice) -> Unit,
    onDismissCode: () -> Unit,
    onToast: (String) -> Unit,
) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    var copied by remember { mutableStateOf(false) }
    var logoutArmed by remember { mutableStateOf(false) }
    LaunchedEffect(copied, logoutArmed) {
        if (copied || logoutArmed) {
            delay(2500)
            copied = false
            logoutArmed = false
        }
    }
    val clipboard = LocalClipboardManager.current
    val codeLeft = ((state.pairCodeExpiresAt - now) / 1000).coerceAtLeast(0)
    val self = state.devices.firstOrNull { it.current }
    val external = state.devices.filterNot { it.current }

    Column(Modifier.fillMaxSize()) {
        AppBar("设备", onBack)
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState())
                .padding(top = 14.dp),
        ) {
            if (state.pairCode.isNotBlank()) {
                Text("一次性配对码", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
                Box(
                    Modifier
                        .fillMaxWidth()
                        .padding(top = 9.dp)
                        .clip(RoundedCornerShape(Dimens.rCard))
                        .background(Tokens.surface)
                        .border(1.dp, Tokens.accent, RoundedCornerShape(Dimens.rCard))
                        .padding(horizontal = 16.dp, vertical = 18.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        state.pairCode,
                        color = Tokens.accentText,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 30.sp,
                        letterSpacing = 5.sp,
                        fontWeight = W650,
                    )
                }
                Row(
                    Modifier.fillMaxWidth().padding(top = 9.dp),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        if (codeLeft > 0) "还剩 ${codeLeft / 60}:${(codeLeft % 60).toString().padStart(2, '0')}，只能用一次" else "已过期，请重新生成",
                        color = if (codeLeft > 0) Tokens.okText else Tokens.warnText,
                        fontSize = 12.sp,
                    )
                    Text(
                        if (copied) "已复制 ✓" else "复制",
                        color = if (copied) Tokens.okText else Tokens.accentText,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                        fontWeight = W650,
                        modifier = Modifier.clickable(role = Role.Button) {
                            clipboard.setText(AnnotatedString(state.pairCode))
                            copied = true
                            onToast("配对码已复制")
                        },
                    )
                }
                Text("在新机器上运行：", color = Tokens.muted, fontSize = 12.sp, modifier = Modifier.padding(top = 14.dp))
                Box(
                    Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp)
                        .clip(RoundedCornerShape(Dimens.rCard))
                        .background(Tokens.surface)
                        .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
                        .padding(horizontal = 14.dp, vertical = 12.dp),
                ) {
                    Text(
                        "easyGet pair --broker ${state.brokerUrl} --code ${state.pairCode}",
                        color = Tokens.fg,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 12.sp,
                        lineHeight = 18.sp,
                    )
                }
                Text(
                    "换来的设备令牌 180 天有效，可随时在这里撤销。",
                    color = Tokens.muted,
                    fontSize = 12.sp,
                    lineHeight = 18.sp,
                    modifier = Modifier.padding(top = 8.dp),
                )
                Spacer(Modifier.height(10.dp))
            }
            SectionTitle("这台手机（审批端）")
            if (self != null) {
                SelfDeviceCard(self, logoutArmed, onRenew = {
                    logoutArmed = false
                    onRenew()
                }, onRename = {
                    logoutArmed = false
                    onRename(self)
                }, onLogout = {
                    if (logoutArmed) {
                        logoutArmed = false
                        onRevoke(self)
                    } else {
                        logoutArmed = true
                    }
                })
            } else {
                Text("未配对", color = Tokens.muted, fontSize = 13.sp)
            }
            SectionTitle("已配对设备", top = 22.dp)
            if (external.isEmpty()) {
                Text(
                    "还没有外部设备。生成配对码，在新机器上运行 easyGet pair。",
                    color = Tokens.muted,
                    fontSize = 13.sp,
                    lineHeight = 18.sp,
                )
            } else {
                HairList {
                    external.forEach { device -> ExternalDeviceRow(device, onRename, onRevoke) }
                }
            }
            Text(
                "外部设备是发起取凭据请求的一方；令牌 180 天有效，随时可撤销。",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(top = 14.dp),
            )
        }
        ActionsColumn {
            when {
                state.pairCode.isNotBlank() && codeLeft <= 0 -> PrimaryButton("重新生成配对码") { onAdd() }
                state.pairCode.isNotBlank() -> QuietButton("收起配对码") { onDismissCode() }
                else -> PrimaryButton("添加设备（生成配对码）") { onAdd() }
            }
        }
    }
}

@Composable
private fun SectionTitle(text: String, top: Dp = 18.dp) {
    Text(
        text.uppercase(),
        color = Tokens.muted,
        fontFamily = FontFamily.Monospace,
        fontSize = 11.sp,
        letterSpacing = 1.1.sp,
        modifier = Modifier.padding(top = top, bottom = 8.dp),
    )
}
