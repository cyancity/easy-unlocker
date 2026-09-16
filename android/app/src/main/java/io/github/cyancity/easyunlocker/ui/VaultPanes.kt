package io.github.cyancity.easyunlocker.ui

import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.cyancity.easyunlocker.UiState
import io.github.cyancity.easyunlocker.crypto.OpenSshKey
import io.github.cyancity.easyunlocker.data.VaultField
import io.github.cyancity.easyunlocker.data.VaultItem

@Composable
fun VaultPane(
    state: UiState,
    scroll: ScrollState,
    onOpen: (String) -> Unit,
    onAdd: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    val q = query.trim().lowercase()
    val filtered = if (q.isEmpty()) {
        state.items
    } else {
        state.items.filter { item ->
            item.name.lowercase().contains(q) || item.aliases.any { it.lowercase().contains(q) }
        }
    }
    Column(Modifier.fillMaxSize()) {
        PageTitle("条目") { AddBtn(onAdd) }
        if (state.items.isNotEmpty()) {
            Box(Modifier.padding(horizontal = Dimens.page)) {
                Field(query, { query = it }, "搜索", mono = false, placeholder = "条目名或别名")
            }
        }
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(scroll),
        ) {
            when {
                state.loading && state.items.isEmpty() -> SkeletonBlock(state.brokerUrl.ifBlank { "broker" })
                state.items.isEmpty() -> EmptyVault(onAdd)
                filtered.isEmpty() -> Text(
                    "没有匹配的条目。",
                    color = Tokens.muted,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(top = 14.dp),
                )
                else -> HairList {
                    filtered.forEachIndexed { i, item ->
                        ListRow(
                            title = item.name,
                            subtitle = "••••••••",
                            last = i == filtered.lastIndex,
                            onClick = { onOpen(item.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun EmptyVault(onAdd: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(top = 48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Icon(UnlockerIcons.Key, contentDescription = null, tint = Tokens.dim, modifier = Modifier.size(30.dp))
        Text("还没有条目", color = Tokens.fg, fontSize = 20.sp, fontWeight = W650, lineHeight = 28.sp, letterSpacing = 0.sp)
        Text(
            "贴一条 Agent 要用的密钥。请求到来时你再手选批准。",
            color = Tokens.muted,
            fontSize = 12.sp,
            lineHeight = 18.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(horizontal = 24.dp),
        )
        QuietButton("添加条目", modifier = Modifier.padding(top = 4.dp).fillMaxWidth(0.55f), onClick = onAdd)
    }
}

@Composable
fun ItemPane(
    item: VaultItem,
    onBack: () -> Unit,
    onMenu: () -> Unit,
    onCopy: (String) -> Unit,
) {
    Column(Modifier.fillMaxSize()) {
        AppBar(item.name, onBack, trailing = { IconBtn(UnlockerIcons.More, "更多操作", onMenu) })
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState())
                .padding(top = 12.dp, bottom = 20.dp),
        ) {
            if (item.aliases.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    item.aliases.forEach { Chip("别名 $it") }
                }
            }
            ValueSection("密码", item.secret) { onCopy(VaultField.PASSWORD) }
            ValueSection("备注", item.note) { onCopy(VaultField.NOTE) }
            val ca = remember(item.note, item.secret) {
                OpenSshKey.parseOrNull(item.note) ?: OpenSshKey.parseOrNull(item.secret)
            }
            if (ca != null) {
                CaKeySection(ca)
            }
            Text(
                "切到后台会自动上锁 · 已屏蔽截屏",
                color = Tokens.muted,
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 14.dp),
            )
        }
    }
}

@Composable
private fun ValueSection(label: String, value: String, onCopy: () -> Unit) {
    val empty = value.isBlank()
    Row(
        Modifier.fillMaxWidth().padding(top = 20.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(label.uppercase(), color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
        if (!empty) CopyChip(onCopy)
    }
    Box(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(Dimens.rCard))
            .background(Tokens.surface)
            .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
            .padding(horizontal = 16.dp, vertical = 16.dp),
    ) {
        Text(
            if (empty) "--" else value,
            color = if (empty) Tokens.muted else Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 15.sp,
            lineHeight = 23.sp,
            letterSpacing = 0.15.sp,
        )
    }
}

/**
 * 条目里存着 OpenSSH ed25519 私钥时，它就能当 SSH CA 用。这里给出 CA 公钥——
 * 把这行加到目标机的 TrustedUserCAKeys，`easyGet ssh <这条>` 就能签证书登录。
 * 公钥不是秘密，复制走普通剪贴板、不做 30 秒清空。
 */
@Composable
private fun CaKeySection(ca: OpenSshKey.Private) {
    val clipboard = LocalClipboardManager.current
    Row(
        Modifier.fillMaxWidth().padding(top = 20.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text("SSH CA 公钥", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
        CopyChip { clipboard.setText(AnnotatedString(ca.authorizedLine())) }
    }
    Box(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(Dimens.rCard))
            .background(Tokens.accentQuiet)
            .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
            .padding(horizontal = 16.dp, vertical = 16.dp),
    ) {
        Column {
            Text(
                ca.authorizedLine(),
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
                lineHeight = 18.sp,
            )
            Text(
                ca.fingerprint(),
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                modifier = Modifier.padding(top = 6.dp),
            )
            Text(
                "把上面那行放进目标机的 TrustedUserCAKeys 文件，这条目就能给 easyGet ssh 签证书。",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 17.sp,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
    }
}

@Composable
private fun CopyChip(onClick: () -> Unit) {
    val shape = RoundedCornerShape(999.dp)
    Box(
        Modifier
            .clip(shape)
            .background(Tokens.raised)
            .border(1.dp, Tokens.borderStrong, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 5.dp)
            .focusRing(999.dp),
    ) {
        Text("复制", color = Tokens.accentText, fontFamily = FontFamily.Monospace, fontSize = 11.sp, fontWeight = W650)
    }
}

@Composable
fun EditPane(
    isNew: Boolean,
    initialName: String,
    initialSecret: String,
    initialNote: String,
    error: String,
    onChange: () -> Unit,
    onCancel: () -> Unit,
    onSave: (String, String, String) -> Unit,
) {
    var name by remember(initialName) { mutableStateOf(initialName) }
    var secret by remember(initialSecret) { mutableStateOf(initialSecret) }
    var note by remember(initialNote) { mutableStateOf(initialNote) }
    Column(Modifier.fillMaxSize()) {
        AppBar(if (isNew) "添加条目" else "编辑条目", onCancel, backLabel = "取消", backIcon = UnlockerIcons.Close)
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState())
                .padding(top = 14.dp),
        ) {
            Field(name, { name = it; onChange() }, "名称", error = error.isNotBlank(), placeholder = "OPENAI_API_KEY")
            Field(secret, { secret = it; onChange() }, "密码 · 短值", error = error.isNotBlank(), placeholder = "粘贴 token / 密码")
            Field(
                note,
                { note = it; onChange() },
                "备注 · 长文本（可空）",
                error = error.isNotBlank(),
                placeholder = "SSH 私钥、整段 .env 放这里",
                multiline = true,
            )
            if (OpenSshKey.looksLikePrivateKey(note)) {
                Text(
                    "备注里是一把 SSH 私钥——这条可以当 CA 给 easyGet ssh 签证书。",
                    color = Tokens.accentText,
                    fontSize = 12.sp,
                    lineHeight = 18.sp,
                    modifier = Modifier.padding(top = 6.dp),
                )
            } else {
                Text(
                    "要当 SSH CA 用的话：",
                    color = Tokens.muted,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(top = 6.dp),
                )
                QuietButton("生成一把 SSH CA 私钥填进备注", modifier = Modifier.padding(top = 8.dp)) {
                    note = OpenSshKey.generate().toPem()
                    onChange()
                }
            }
            if (error.isNotBlank()) {
                Text(error, color = Tokens.dangerText, fontSize = 12.sp, fontWeight = W650, lineHeight = 18.sp)
            } else {
                Text("和 easyGet 里的请求名可以不同，审批时手选对齐。", color = Tokens.muted, fontSize = 12.sp, lineHeight = 18.sp)
            }
        }
        ActionsColumn {
            PrimaryButton("保存") { onSave(name, secret, note) }
        }
    }
}

@Composable
fun ItemMenuSheet(onEdit: () -> Unit, onDelete: () -> Unit) {
    Text("条目操作", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 12.dp))
    HairList {
        Box(
            Modifier
                .fillMaxWidth()
                .height(60.dp)
                .clickable(role = Role.Button, onClick = onEdit)
                .padding(vertical = 12.dp),
            contentAlignment = Alignment.CenterStart,
        ) {
            Text("编辑名称、密码与备注", color = Tokens.fg, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650)
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
            Text("删除这条凭据", color = Tokens.dangerText, fontFamily = FontFamily.Monospace, fontSize = 15.sp, fontWeight = W650)
        }
    }
}

@Composable
fun ConfirmDeleteSheet(name: String, onConfirm: () -> Unit, onCancel: () -> Unit) {
    Text("删除 $name？", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(horizontal = 6.dp))
    Text(
        "删除后无法撤销。恢复只能靠密文备份 + 恢复码。",
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
        Text("删除", color = Tokens.onDanger, fontSize = 16.sp, fontWeight = W650)
    }
    Spacer(Modifier.height(10.dp))
    QuietButton("取消", onClick = onCancel)
}
