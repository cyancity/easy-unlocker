package io.github.cyancity.easyunlocker.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.ui.graphics.graphicsLayer
import kotlin.random.Random
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.cyancity.easyunlocker.UiState
import io.github.cyancity.easyunlocker.crypto.OpenSshKey
import io.github.cyancity.easyunlocker.crypto.SshCert
import io.github.cyancity.easyunlocker.data.Delivery
import io.github.cyancity.easyunlocker.data.PendingRequest
import io.github.cyancity.easyunlocker.data.ReservedItem
import io.github.cyancity.easyunlocker.data.VaultField
import io.github.cyancity.easyunlocker.data.VaultItem
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@Composable
fun PendingPane(
    state: UiState,
    onChange: () -> Unit,
    onSelectField: (String) -> Unit,
    onRemember: (Boolean) -> Unit,
    onApprove: () -> Unit,
    onApprovePassword: (String) -> Unit,
    onDeny: () -> Unit,
    onRetry: () -> Unit,
    onExpire: (String) -> Unit,
    onArrived: () -> Unit,
    onOpenGateway: (String) -> Unit,
) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(1000)
            now = System.currentTimeMillis()
        }
    }
    val live = state.pending.filter { it.remainingSeconds(now) > 0 }
    val expired = state.pending.filter { it.remainingSeconds(now) <= 0 }
    LaunchedEffect(expired.map { it.request_id }.joinToString()) {
        expired.forEach { onExpire(it.request_id) }
    }
    val req = live.firstOrNull() ?: state.selectedPending?.takeIf { it.remainingSeconds(now) > 0 }
    // 保留名请求（#items）没有条目可选、没有值可放，整页自己一套渲染。
    val listRequest = req != null && ReservedItem.isListRequest(req.item)
    // sign 请求要签发证书而不是放出值，也是自己一套渲染。
    val signRequest = req != null && req.mode == "sign"
    Column(Modifier.fillMaxSize()) {
        if (req != null) {
            val remain = req.remainingSeconds(now)
            val total = req.ttl.coerceAtLeast(1)
            val pct = (remain.toFloat() / total).coerceIn(0.06f, 1f)
            ArrivalBar(pct = pct, play = state.incoming, onPlayed = onArrived)
        }
        PageTitle("待批准") { if (state.paired) Chip("网关 ${state.deviceName}") }
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .verticalScroll(rememberScrollState()),
        ) {
            if (state.offline) {
                OfflineBanner(state.lastSync, onRetry)
                Spacer(Modifier.height(16.dp))
            }
            if (state.otherPending.isNotEmpty()) {
                OtherGatewayHint(state, onOpenGateway)
            }
            when {
                state.loading -> SkeletonBlock(state.brokerUrl.ifBlank { "broker" })
                req == null -> EmptyPending(state.timedOut)
                else -> {
                    val reduce = LocalReduceMotion.current
                    val enter = remember(req.request_id) { Animatable(if (state.incoming && !reduce) 0f else 1f) }
                    LaunchedEffect(req.request_id, state.incoming) {
                        if (state.incoming && !reduce) {
                            enter.snapTo(0f)
                            enter.animateTo(1f, tween(560, easing = CubicBezierEasing(0.2f, 0f, 0f, 1f)))
                        } else {
                            enter.snapTo(1f)
                        }
                    }
                    Box(
                        Modifier.graphicsLayer {
                            alpha = enter.value
                            translationY = (1f - enter.value) * 28f
                        },
                    ) {
                        if (listRequest) {
                            ListBody(state, req, now)
                        } else if (signRequest) {
                            SignBody(state, req, now, onChange)
                        } else {
                            PendingBody(state, req, now, onChange, onSelectField, onRemember)
                        }
                    }
                }
            }
        }
        if (!state.loading && listRequest) {
            ActionsColumn {
                PrimaryButton(
                    "允许列出（${state.items.size} 条）",
                    enabled = state.items.isNotEmpty(),
                    onClick = onApprove,
                )
                PasswordApprove(state.hasPassword, onApprovePassword)
                DeclineButton("拒绝这次请求", onDeny)
            }
        }
        if (!state.loading && req != null && signRequest) {
            val chosen = state.items.firstOrNull { it.id == req.selectedItemId }
            val caReady = chosen != null && caKeyOf(chosen) != null
            ActionsColumn {
                PrimaryButton(
                    "签发证书（${fmtTtl(req.cert_ttl)}）",
                    enabled = caReady,
                    onClick = onApprove,
                )
                PasswordApprove(state.hasPassword, onApprovePassword)
                DeclineButton("拒绝这次请求", onDeny)
            }
        }
        if (!state.loading && req != null && !listRequest && !signRequest) {
            val chosen = state.items.firstOrNull { it.id == req.selectedItemId }
            val field = chosen?.let { req.selectedField ?: VaultField.defaultFor(it.secret, it.note) }
            val ready = chosen != null &&
                if (field == VaultField.NOTE) chosen.note.isNotBlank() else chosen.secret.isNotBlank()
            ActionsColumn {
                PrimaryButton(
                    if (req.delivery == Delivery.EPHEMERAL) "放出（不落盘）" else "确认写入磁盘",
                    enabled = ready,
                    onClick = onApprove,
                )
                PasswordApprove(state.hasPassword, onApprovePassword)
                DeclineButton("拒绝这次请求", onDeny)
            }
        }
    }
}

/**
 * 批准页的生物识别兜底：设了解锁密码时出现「用密码批准」，展开一个密码框。
 * 密码本身就完成了认证，所以这条路不再弹指纹。
 */
@Composable
private fun PasswordApprove(hasPassword: Boolean, onApprovePassword: (String) -> Unit) {
    if (!hasPassword) return
    var show by remember { mutableStateOf(false) }
    var password by remember { mutableStateOf("") }
    if (show) {
        Field(password, { password = it }, "解锁密码", placeholder = "输入解锁密码", secret = true)
        QuietButton("确认批准") { onApprovePassword(password) }
    } else {
        QuietButton("用密码批准") { show = true }
    }
}

/** 「命令」字段：还原当时那条 CLI，三种请求 body 共用（对齐 iOS ReqField("命令")）。 */
@Composable
private fun CommandSection(cmd: String) {
    Text(
        "命令",
        color = Tokens.muted,
        fontFamily = FontFamily.Monospace,
        fontSize = 11.sp,
        letterSpacing = 1.1.sp,
        modifier = Modifier.padding(top = 20.dp, bottom = 10.dp),
    )
    CommandBlock(cmd)
}

private data class Spark(
    val x: Float,
    val y: Float,
    val vx: Float,
    val vy: Float,
    val life: Float,
    val size: Float,
)

@Composable
private fun ArrivalBar(pct: Float, play: Boolean, onPlayed: () -> Unit) {
    val reduce = LocalReduceMotion.current
    val shown = remember { Animatable(pct.coerceIn(0.06f, 1f)) }
    val green = Tokens.ok
    val track = Tokens.raised
    var sparks by remember { mutableStateOf(emptyList<Spark>()) }
    val ease = CubicBezierEasing(0.2f, 0f, 0f, 1f)

    LaunchedEffect(play) {
        if (play) {
            if (!reduce) {
                shown.snapTo(0.04f)
                shown.animateTo(pct.coerceIn(0.06f, 1f), tween(720, easing = ease))
                val rnd = Random(System.nanoTime())
                repeat(40) {
                    val tip = shown.value
                    sparks = (
                        sparks.map {
                            it.copy(x = it.x + it.vx, y = it.y + it.vy, vy = it.vy + 0.045f, life = it.life - 0.05f)
                        }.filter { it.life > 0 } + List(5) {
                            Spark(
                                x = tip,
                                y = 0f,
                                vx = (rnd.nextFloat() - 0.2f) * 0.018f,
                                vy = -0.07f - rnd.nextFloat() * 0.14f,
                                life = 0.65f + rnd.nextFloat() * 0.35f,
                                size = 1.4f + rnd.nextFloat() * 2.2f,
                            )
                        }
                        ).take(90)
                    delay(16)
                }
                sparks = emptyList()
            } else {
                shown.snapTo(pct.coerceIn(0.06f, 1f))
            }
            onPlayed()
        }
    }
    LaunchedEffect(pct) {
        if (!play) shown.snapTo(pct.coerceIn(0.06f, 1f))
    }

    val barPct = shown.value
    Canvas(Modifier.fillMaxWidth().height(14.dp)) {
        val h = 3.dp.toPx()
        val mid = size.height / 2f
        drawRect(track, topLeft = Offset(0f, mid - h / 2f), size = Size(size.width, h))
        drawRect(green, topLeft = Offset(0f, mid - h / 2f), size = Size(size.width * barPct, h))
        sparks.forEach { s ->
            drawCircle(
                color = green.copy(alpha = s.life.coerceIn(0f, 1f)),
                radius = s.size,
                center = Offset(s.x * size.width, mid + s.y * size.height),
            )
        }
    }
}

/**
 * 别的网关上有待批准。批准只能在收到请求的那台上做，所以这里只给「切过去」——
 * 切过去会自动把那条请求展开。
 */
@Composable
private fun OtherGatewayHint(state: UiState, onOpen: (String) -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(bottom = 16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.otherPending.forEach { other ->
            val pairing = state.pairings.firstOrNull { it.id == other.pairingId } ?: return@forEach
            Row(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(Dimens.rCard))
                    .background(Tokens.surface)
                    .border(1.dp, Tokens.warn, RoundedCornerShape(Dimens.rCard))
                    .padding(horizontal = 14.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Column(Modifier.weight(1f)) {
                    Text(
                        "${pairing.name} 有 ${other.count} 条待批准",
                        color = Tokens.fg,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Text(
                        "请求在另一台网关上，得切过去才能批",
                        color = Tokens.muted,
                        fontSize = 11.sp,
                        lineHeight = 16.sp,
                        modifier = Modifier.padding(top = 3.dp),
                    )
                }
                Text(
                    "切过去",
                    color = Tokens.accentText,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clip(RoundedCornerShape(999.dp))
                        .clickable(role = Role.Button) { onOpen(other.pairingId) }
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                )
            }
        }
    }
}

@Composable
private fun EmptyPending(timedOut: Boolean) {
    Column(
        Modifier.fillMaxWidth().padding(top = 36.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Icon(UnlockerIcons.Bell, contentDescription = null, tint = Tokens.dim, modifier = Modifier.size(28.dp))
        if (timedOut) Chip("已超时")
        Text(
            "没有等待你的请求。App 开着时新的请求会自动出现在这里。",
            color = Tokens.muted,
            fontSize = 12.sp,
            lineHeight = 18.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(horizontal = 20.dp),
        )
    }
}

@Composable
private fun PendingBody(
    state: UiState,
    req: PendingRequest,
    now: Long,
    onChange: () -> Unit,
    onSelectField: (String) -> Unit,
    onRemember: (Boolean) -> Unit,
) {
    val chosen = state.items.firstOrNull { it.id == req.selectedItemId }
    val exact = chosen != null && namesOf(chosen).contains(req.item.lowercase())
    val unmatched = chosen == null
    Column(Modifier.fillMaxWidth().padding(top = 52.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(
                "${req.requester} 想取走",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                modifier = Modifier.weight(1f),
            )
            Text(
                fmt(req.remainingSeconds(now)),
                color = Tokens.okText,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
            )
        }
        Text(
            req.item,
            color = Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 34.sp,
            fontWeight = W650,
            lineHeight = 36.sp,
            letterSpacing = (-1.19).sp,
            modifier = Modifier.padding(top = 20.dp),
        )
        if (req.purpose.isNotBlank()) {
            Text(req.purpose, color = Tokens.fg, fontSize = 17.sp, lineHeight = 25.sp, modifier = Modifier.padding(top = 22.dp))
        }
        if (req.target.isNotBlank()) {
            Text("写入 ${req.target}", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 13.sp, modifier = Modifier.padding(top = 6.dp))
        }
        CommandSection(req.commandLine)
        Spacer(Modifier.height(28.dp))
        Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            Text("放出这一条", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
            Text(
                when {
                    unmatched -> "需要你指定"
                    exact -> "完全匹配"
                    else -> "已手选"
                },
                color = if (unmatched) Tokens.warnText else Tokens.accentText,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                letterSpacing = 0.44.sp,
            )
        }
        val shape = RoundedCornerShape(14.dp)
        Row(
            Modifier
                .fillMaxWidth()
                .height(60.dp)
                .clip(shape)
                .background(Tokens.surface)
                .border(1.dp, if (unmatched) Tokens.warn else Tokens.borderStrong, shape)
                .clickable(role = Role.Button, onClick = onChange)
                .padding(horizontal = 16.dp)
                .focusRing(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                chosen?.name ?: "没有完全匹配的条目",
                color = if (unmatched) Tokens.warnText else Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f),
            )
            Text(
                if (chosen != null) "改" else "选择",
                color = when {
                    unmatched -> Tokens.warnText
                    else -> Tokens.accentText
                },
                fontSize = 12.sp,
            )
            Icon(
                UnlockerIcons.Chevron,
                contentDescription = null,
                tint = if (unmatched) Tokens.warnText else Tokens.muted,
                modifier = Modifier.size(15.dp),
            )
        }
        if (chosen != null) {
            val field = req.selectedField ?: VaultField.defaultFor(chosen.secret, chosen.note)
            Row(
                Modifier.fillMaxWidth().padding(top = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text("放出", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
                FieldToggle(field, onSelectField)
            }
            SecretPreview(if (field == VaultField.NOTE) chosen.note else chosen.secret)
        }
        DeliveryNotice(req.delivery, req.target)
        if (state.items.isEmpty()) {
            Text("库里还没有条目，先去添加一条再回来。", color = Tokens.muted, fontSize = 12.sp, modifier = Modifier.padding(top = 16.dp))
        }
        if (chosen != null && chosen.name.lowercase() != req.item.lowercase()) {
            Row(
                Modifier
                    .fillMaxWidth()
                    .height(52.dp)
                    .clickable(role = Role.Button, onClick = { onRemember(!state.rememberAlias) })
                    .padding(top = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                SwitchTrack(state.rememberAlias)
                Text(
                    "把 ${req.item} 记成这个名字的别名",
                    color = Tokens.fg,
                    fontSize = 13.sp,
                )
            }
        }
        Spacer(Modifier.height(26.dp))
    }
}

/** 这条目能不能当 CA：密码或备注栏里要有一把 OpenSSH ed25519 私钥。 */
private fun caKeyOf(item: VaultItem): OpenSshKey.Private? =
    OpenSshKey.parseOrNull(item.note) ?: OpenSshKey.parseOrNull(item.secret)

/** 证书有效期显示成「5 分钟 / 1 小时 / 45 秒」。 */
private fun fmtTtl(seconds: Int): String {
    val ttl = if (seconds > 0) seconds else SshCert.DEFAULT_TTL
    return when {
        ttl < 60 -> "$ttl 秒"
        ttl < 3600 -> "${ttl / 60} 分钟"
        ttl < 86400 -> "${ttl / 3600} 小时"
        else -> "${ttl / 86400} 天"
    }
}

/**
 * sign 请求：CLI 要一张短时 SSH 证书，不放出任何值。
 * 选中的条目当 CA——它的密码/备注栏里要有一把 OpenSSH ed25519 私钥。
 */
@Composable
private fun SignBody(
    state: UiState,
    req: PendingRequest,
    now: Long,
    onChange: () -> Unit,
) {
    val chosen = state.items.firstOrNull { it.id == req.selectedItemId }
    val chosenKey = chosen?.let { caKeyOf(it) }
    Column(Modifier.fillMaxWidth().padding(top = 52.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(
                "${req.requester} 想签发",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                modifier = Modifier.weight(1f),
            )
            Text(
                fmt(req.remainingSeconds(now)),
                color = Tokens.okText,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
            )
        }
        Text(
            "SSH 证书",
            color = Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 34.sp,
            fontWeight = W650,
            lineHeight = 36.sp,
            letterSpacing = (-1.19).sp,
            modifier = Modifier.padding(top = 20.dp),
        )
        if (req.purpose.isNotBlank()) {
            Text(req.purpose, color = Tokens.fg, fontSize = 17.sp, lineHeight = 25.sp, modifier = Modifier.padding(top = 22.dp))
        }
        Spacer(Modifier.height(24.dp))
        Column(
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(14.dp))
                .background(Tokens.surface)
                .border(1.dp, Tokens.border, RoundedCornerShape(14.dp))
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SignInfoRow("登录用户", req.ssh_user)
            SignInfoRow("证书有效", fmtTtl(req.cert_ttl) + "（只在登录握手时校验）")
            SignInfoRow("身份公钥", runCatching { OpenSshKey.fingerprintOf(req.public_key) }.getOrNull() ?: "解析失败")
        }
        CommandSection(req.commandLine)
        Spacer(Modifier.height(20.dp))
        Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            Text("用这条 CA 签", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
            Text(
                when {
                    chosen == null -> "需要你指定"
                    chosenKey == null -> "不是 CA"
                    else -> "完全匹配"
                },
                color = if (chosenKey == null) Tokens.warnText else Tokens.accentText,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                letterSpacing = 0.44.sp,
            )
        }
        val shape = RoundedCornerShape(14.dp)
        Row(
            Modifier
                .fillMaxWidth()
                .height(60.dp)
                .clip(shape)
                .background(Tokens.surface)
                .border(1.dp, if (chosenKey == null) Tokens.warn else Tokens.borderStrong, shape)
                .clickable(role = Role.Button, onClick = onChange)
                .padding(horizontal = 16.dp)
                .focusRing(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(
                    chosen?.name ?: "选择当 CA 的条目",
                    color = if (chosen != null) Tokens.fg else Tokens.warnText,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                )
                if (chosenKey != null) {
                    Text(
                        chosenKey.fingerprint(),
                        color = Tokens.muted,
                        fontFamily = FontFamily.Monospace,
                        fontSize = 11.sp,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
            Text(
                if (chosen != null) "改" else "选择",
                color = when {
                    chosen != null -> Tokens.accentText
                    else -> Tokens.warnText
                },
                fontSize = 12.sp,
            )
            Icon(
                UnlockerIcons.Chevron,
                contentDescription = null,
                tint = if (chosen != null) Tokens.muted else Tokens.warnText,
                modifier = Modifier.size(15.dp),
            )
        }
        if (chosen != null && chosenKey == null) {
            Text(
                "这条的密码/备注里没有 OpenSSH ed25519 私钥，当不了 CA。",
                color = Tokens.warnText,
                fontSize = 12.sp,
                lineHeight = 17.sp,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        if (state.items.isEmpty()) {
            Text("库里还没有条目，先去添加一条 CA 私钥再回来。", color = Tokens.muted, fontSize = 12.sp, modifier = Modifier.padding(top = 16.dp))
        }
        SignNotice(req)
        Spacer(Modifier.height(26.dp))
    }
}

@Composable
private fun SignInfoRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(label, color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
        Text(
            value,
            color = Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            textAlign = TextAlign.End,
            modifier = Modifier.padding(start = 16.dp),
        )
    }
}

@Composable
private fun SignNotice(req: PendingRequest) {
    val shape = RoundedCornerShape(12.dp)
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 12.dp)
            .clip(shape)
            .background(Tokens.accentQuiet)
            .padding(horizontal = 14.dp, vertical = 12.dp),
    ) {
        Text("CA 私钥不出手机", color = Tokens.accentText, fontSize = 12.sp, fontWeight = W650)
        Text(
            "对方拿到的只是一张 ${fmtTtl(req.cert_ttl)}内可登录 ${req.ssh_user} 的证书；CA 私钥本身不会被放出。证书只在登录那一刻校验，已建立的会话不受影响。",
            color = Tokens.accentText,
            fontSize = 12.sp,
            lineHeight = 17.sp,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

/**
 * 保留名请求（#items）：CLI 想拿到「这台机器一共有哪些条目名」。
 *
 * 没有条目可选、没有栏可切、也没有值可放——批准出去的就只是一份名字。所以这一页
 * 自己一套渲染，不复用 PendingBody 的选条目/选栏/落盘提示。
 */
@Composable
private fun ListBody(state: UiState, req: PendingRequest, now: Long) {
    Column(Modifier.fillMaxWidth().padding(top = 52.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Text(
                "${req.requester} 想看看",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                modifier = Modifier.weight(1f),
            )
            Text(
                fmt(req.remainingSeconds(now)),
                color = Tokens.okText,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
            )
        }
        Text(
            "条目名名单",
            color = Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 34.sp,
            fontWeight = W650,
            lineHeight = 36.sp,
            letterSpacing = (-1.19).sp,
            modifier = Modifier.padding(top = 20.dp),
        )
        if (req.purpose.isNotBlank()) {
            Text(req.purpose, color = Tokens.fg, fontSize = 17.sp, lineHeight = 25.sp, modifier = Modifier.padding(top = 22.dp))
        }
        CommandSection(req.commandLine)
        Spacer(Modifier.height(28.dp))
        Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalArrangement = Arrangement.SpaceBetween) {
            Text("会告诉它什么", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, letterSpacing = 1.1.sp)
            Text(
                "${state.items.size} 个名字",
                color = Tokens.accentText,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                letterSpacing = 0.44.sp,
            )
        }
        NamePreview(state.items)
        ListNotice()
        Spacer(Modifier.height(26.dp))
    }
}

/** 名单预览最多列几条；再多只报个数。 */
private const val NAME_PREVIEW_CAP = 8

@Composable
private fun NamePreview(items: List<VaultItem>) {
    val shape = RoundedCornerShape(12.dp)
    Column(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(Tokens.surface)
            .border(1.dp, Tokens.border, shape)
            .padding(horizontal = 14.dp, vertical = 12.dp),
    ) {
        if (items.isEmpty()) {
            // 锁着的时候 UiState 里也是空的，所以这句话得同时说清两种可能。
            Text(
                "读不到条目：库可能是空的，也可能还没解锁。",
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 12.sp,
            )
        }
        items.take(NAME_PREVIEW_CAP).forEach { item ->
            Text(
                item.name,
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                lineHeight = 20.sp,
            )
            if (item.aliases.isNotEmpty()) {
                Text(
                    "别名 ${item.aliases.joinToString(", ")}",
                    color = Tokens.muted,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 11.sp,
                    lineHeight = 17.sp,
                )
            }
        }
        if (items.size > NAME_PREVIEW_CAP) {
            Text(
                "…还有 ${items.size - NAME_PREVIEW_CAP} 条",
                color = Tokens.muted,
                fontSize = 11.sp,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
    }
}

@Composable
private fun ListNotice() {
    val shape = RoundedCornerShape(12.dp)
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 12.dp)
            .clip(shape)
            .background(Tokens.accentQuiet)
            .padding(horizontal = 14.dp, vertical = 12.dp),
    ) {
        Text("只给名字，不给任何值", color = Tokens.accentText, fontSize = 12.sp, fontWeight = W650)
        Text(
            "这台机器会知道有哪几条，但每条取用仍然要你单独批准一次；批准名单本身放不出任何密钥。",
            color = Tokens.accentText,
            fontSize = 12.sp,
            lineHeight = 17.sp,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

/** 批准页上最多预览多少个字符；再长就截断，只报总长度。 */
private const val PREVIEW_CAP = 240

/** 让用户在批准前看清「这一栏里到底是什么」，超长只显示开头。 */
@Composable
private fun SecretPreview(value: String) {
    val shape = RoundedCornerShape(12.dp)
    val blank = value.isBlank()
    val truncated = value.length > PREVIEW_CAP
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 8.dp)
            .clip(shape)
            .background(Tokens.surface)
            .border(1.dp, Tokens.border, shape)
            .padding(horizontal = 14.dp, vertical = 12.dp),
    ) {
        Text(
            if (blank) "--" else value.take(PREVIEW_CAP),
            color = if (blank) Tokens.muted else Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            lineHeight = 18.sp,
            maxLines = 4,
            overflow = TextOverflow.Ellipsis,
        )
    }
    if (!blank) {
        Text(
            buildString {
                append("共 ${value.length} 字符")
                val lines = value.count { it == '\n' } + 1
                if (lines > 1) append(" · $lines 行")
                if (truncated) append(" · 只显示开头")
            },
            color = Tokens.muted,
            fontSize = 11.sp,
            modifier = Modifier.padding(top = 6.dp),
        )
    }
}

@Composable
private fun DeliveryNotice(delivery: String, target: String) {
    val toFile = delivery != Delivery.EPHEMERAL
    val shape = RoundedCornerShape(12.dp)
    val bg = if (toFile) Tokens.warnQuiet else Tokens.accentQuiet
    val fg = if (toFile) Tokens.warnText else Tokens.accentText
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 12.dp)
            .clip(shape)
            .background(bg)
            .padding(horizontal = 14.dp, vertical = 12.dp),
    ) {
        Text(if (toFile) "会落盘" else "不落盘，用完即焚", color = fg, fontSize = 12.sp, fontWeight = W650)
        Text(
            if (toFile) {
                val where = target.ifBlank { "请求方给的路径" }
                "$where 会被写入文件，内容留在那台机器上，直到你手动删掉。"
            } else {
                "值只交给请求方进程（环境变量 / 匿名 fd），不写文件；进程退出即消失。"
            },
            color = fg,
            fontSize = 12.sp,
            lineHeight = 17.sp,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

@Composable
fun PickerSheet(items: List<VaultItem>, onChoose: (String) -> Unit) {
    Text("放出哪一条", color = Tokens.fg, fontSize = 17.sp, fontWeight = W650, lineHeight = 24.sp, modifier = Modifier.padding(start = 6.dp, end = 6.dp, bottom = 12.dp))
    HairList {
        items.forEachIndexed { i, it ->
            Column(
                Modifier
                    .fillMaxWidth()
                    .clickable(role = Role.Button, onClick = { onChoose(it.id) })
                    .padding(vertical = 12.dp),
            ) {
                Text(it.name, color = Tokens.fg, fontFamily = FontFamily.Monospace, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
                Text("••••••••", color = Tokens.muted, fontFamily = FontFamily.Monospace, fontSize = 13.sp, modifier = Modifier.padding(top = 4.dp))
                if (it.aliases.isNotEmpty()) {
                    Row(Modifier.padding(top = 6.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Chip("别名 ${it.aliases.first()}")
                    }
                }
            }
            if (i != items.lastIndex) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
        }
    }
}

@Composable
fun ApprovedPane(item: String, requester: String, onBack: () -> Unit) {
    val list = ReservedItem.isListRequest(item)
    val reduce = LocalReduceMotion.current
    val ease = CubicBezierEasing(0.2f, 0f, 0f, 1f)
    val ring = remember { Animatable(if (reduce) 1f else 0f) }
    val circle = remember { Animatable(if (reduce) 1f else 0f) }
    val check = remember { Animatable(if (reduce) 1f else 0f) }
    val text = remember { Animatable(if (reduce) 1f else 0f) }
    LaunchedEffect(Unit) {
        if (reduce) return@LaunchedEffect
        launch { ring.animateTo(1f, tween(620, easing = CubicBezierEasing(0.22f, 0.9f, 0.28f, 1f))) }
        launch { circle.animateTo(1f, tween(700, easing = ease)) }
        launch {
            delay(260)
            check.animateTo(1f, tween(440, easing = ease))
        }
        launch {
            delay(460)
            text.animateTo(1f, tween(440, easing = LinearEasing))
        }
    }
    val accent = Tokens.accent
    Column(Modifier.fillMaxSize()) {
        Column(
            Modifier.weight(1f).fillMaxWidth().padding(horizontal = Dimens.page),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Box(Modifier.size(96.dp), contentAlignment = Alignment.Center) {
                Canvas(Modifier.fillMaxSize()) {
                    val ringScale = 0.55f + 0.87f * ring.value
                    val ringAlpha = 0.55f * (1f - ring.value)
                    scale(ringScale) {
                        drawCircle(
                            color = accent.copy(alpha = ringAlpha),
                            style = Stroke(2.dp.toPx()),
                        )
                    }
                    val stroke = Stroke(2.4.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round)
                    val r = size.minDimension * (26f / 96f)
                    drawArc(
                        color = accent,
                        startAngle = -90f,
                        sweepAngle = 360f * circle.value,
                        useCenter = false,
                        style = stroke,
                        topLeft = androidx.compose.ui.geometry.Offset(size.width / 2 - r, size.height / 2 - r),
                        size = androidx.compose.ui.geometry.Size(r * 2, r * 2),
                    )
                    val checkPath = Path().apply {
                        moveTo(size.width * 32f / 96f, size.height * 49f / 96f)
                        lineTo(size.width * 43f / 96f, size.height * 60f / 96f)
                        lineTo(size.width * 65f / 96f, size.height * 36f / 96f)
                    }
                    val measure = PathMeasure()
                    measure.setPath(checkPath, false)
                    val dst = Path()
                    measure.getSegment(0f, measure.length * check.value, dst, true)
                    drawPath(dst, accent, style = stroke)
                }
            }
            Column(
                Modifier
                    .alpha(text.value)
                    .offset(y = (10.dp * (1f - text.value))),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text("已放行", color = Tokens.fg, fontSize = 34.sp, fontWeight = W650, lineHeight = 45.sp, letterSpacing = 0.sp)
                Text(
                    if (list) "名单 → $requester" else "$item → $requester",
                    color = Tokens.muted,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(top = 9.dp),
                )
                Text(
                    if (list) {
                        "只有条目名，没有任何值离开手机。"
                    } else {
                        "值已用本次 ask 的临时公钥密封，Broker 只转发、解不开。"
                    },
                    color = Tokens.muted,
                    fontSize = 12.sp,
                    lineHeight = 18.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 13.dp).fillMaxWidth(0.8f),
                )
            }
        }
        ActionsColumn {
            QuietButton("返回条目", onClick = onBack)
        }
    }
}

private fun namesOf(item: VaultItem): List<String> =
    listOf(item.name.lowercase()) + item.aliases.map { it.lowercase() }

private fun fmt(seconds: Int): String {
    val s = seconds.coerceAtLeast(0)
    return "${s / 60}:${(s % 60).toString().padStart(2, '0')}"
}
