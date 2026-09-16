package io.github.cyancity.easyunlocker.ui

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import io.github.cyancity.easyunlocker.Screen
import io.github.cyancity.easyunlocker.data.VaultField

@Composable
fun Modifier.focusRing(radius: Dp = Dimens.rButton): Modifier {
    val fg = Tokens.fg
    var focused by remember { mutableStateOf(false) }
    return this
        .onFocusChanged { focused = it.isFocused }
        .drawWithContent {
            drawContent()
            if (focused) {
                val pad = 3.dp.toPx()
                drawRoundRect(
                    color = fg,
                    topLeft = androidx.compose.ui.geometry.Offset(-pad, -pad),
                    size = androidx.compose.ui.geometry.Size(this.size.width + pad * 2, this.size.height + pad * 2),
                    cornerRadius = CornerRadius(radius.toPx() + pad, radius.toPx() + pad),
                    style = Stroke(width = 2.dp.toPx()),
                )
            }
        }
}

@Composable
fun PageTitle(title: String, trailing: @Composable (() -> Unit)? = null) {
    Row(
        Modifier.fillMaxWidth().padding(start = Dimens.page, end = Dimens.page, top = 4.dp, bottom = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(title, color = Tokens.fg, fontSize = 32.sp, fontWeight = W650, lineHeight = 42.sp, letterSpacing = 0.sp)
        trailing?.invoke()
    }
}

@Composable
fun AppBar(
    title: String,
    onBack: () -> Unit,
    backLabel: String = "返回",
    backIcon: ImageVector = UnlockerIcons.Back,
    trailing: @Composable (() -> Unit)? = null,
) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp).height(56.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconBtn(backIcon, backLabel, onBack)
        Text(
            title,
            color = Tokens.fg,
            fontFamily = FontFamily.Monospace,
            fontSize = 16.sp,
            fontWeight = FontWeight.SemiBold,
            letterSpacing = (-0.32).sp,
            modifier = Modifier.weight(1f).padding(horizontal = 4.dp),
            maxLines = 2,
        )
        trailing?.invoke() ?: Spacer(Modifier.size(Dimens.touch))
    }
}

@Composable
fun IconBtn(icon: ImageVector, label: String, onClick: () -> Unit, tint: Color = Tokens.fg) {
    Box(
        Modifier
            .size(Dimens.touch)
            .clip(CircleShape)
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { contentDescription = label }
            .focusRing(24.dp),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(20.dp))
    }
}

@Composable
fun AddBtn(onClick: () -> Unit) {
    Box(
        Modifier
            .size(Dimens.touch)
            .clip(CircleShape)
            .background(Tokens.accent)
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { contentDescription = "添加条目" }
            .focusRing(24.dp),
        contentAlignment = Alignment.Center,
    ) {
        Icon(UnlockerIcons.Plus, contentDescription = null, tint = Tokens.onAccent, modifier = Modifier.size(20.dp))
    }
}

@Composable
fun PrimaryButton(
    text: String,
    enabled: Boolean = true,
    modifier: Modifier = Modifier,
    capsule: Boolean = false,
    onClick: () -> Unit,
) {
    val radius = if (capsule) 999.dp else Dimens.rButton
    val shape = RoundedCornerShape(radius)
    Box(
        modifier
            .fillMaxWidth()
            .height(Dimens.mainButton)
            .clip(shape)
            .background(if (enabled) Tokens.accent else Tokens.raised)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .focusRing(radius),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = if (enabled) Tokens.onAccent else Tokens.muted, fontSize = 16.sp, fontWeight = W650)
    }
}

@Composable
fun QuietButton(text: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val shape = RoundedCornerShape(Dimens.rButton)
    Box(
        modifier
            .fillMaxWidth()
            .height(48.dp)
            .clip(shape)
            .border(1.dp, Tokens.borderStrong, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .focusRing(Dimens.rButton),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = Tokens.fg, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun DeclineButton(text: String, onClick: () -> Unit) {
    Box(
        Modifier
            .fillMaxWidth()
            .height(46.dp)
            .clip(RoundedCornerShape(12.dp))
            .clickable(role = Role.Button, onClick = onClick)
            .focusRing(12.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = Tokens.muted, fontSize = 15.sp, fontWeight = W550)
    }
}

@Composable
fun ActionsColumn(content: @Composable () -> Unit) {
    Column(
        Modifier
            .fillMaxWidth()
            .padding(start = Dimens.page, end = Dimens.page, top = 16.dp, bottom = Dimens.actionBottom),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.hair))
        Spacer(Modifier.height(14.dp))
        content()
    }
}

@Composable
fun Field(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    mono: Boolean = true,
    error: Boolean = false,
    placeholder: String = "",
    multiline: Boolean = false,
    secret: Boolean = false,
) {
    // 输入框的焦点提示就是「紫色描边 + 外圈柔光」，不要再叠 focusRing（那圈是给按钮的，
    // 画在输入框上会变成一圈多余的深色边框）。
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val shape = RoundedCornerShape(Dimens.rField)
    val accentSoft = Tokens.accentSoft
    val fieldRadius = Dimens.rField
    val border = when {
        error -> Tokens.danger
        focused -> Tokens.accent
        else -> Tokens.borderStrong
    }
    Column(Modifier.fillMaxWidth().padding(bottom = 22.dp)) {
        Text(
            label.uppercase(java.util.Locale.US),
            color = Tokens.muted,
            fontFamily = FontFamily.Monospace,
            fontSize = 11.sp,
            letterSpacing = 1.1.sp,
            modifier = Modifier.padding(bottom = 9.dp),
        )
        BasicTextField(
            value = value,
            onValueChange = onChange,
            singleLine = !multiline,
            visualTransformation = if (secret) PasswordVisualTransformation() else VisualTransformation.None,
            keyboardOptions = if (secret) KeyboardOptions(keyboardType = KeyboardType.Password) else KeyboardOptions.Default,
            textStyle = TextStyle(
                color = Tokens.fg,
                fontSize = if (mono) 14.sp else 16.sp,
                fontFamily = if (mono) FontFamily.Monospace else FontFamily.SansSerif,
                letterSpacing = if (mono) (-0.14).sp else 0.sp,
                lineHeight = if (multiline) 21.sp else 20.sp,
            ),
            cursorBrush = SolidColor(Tokens.accent),
            interactionSource = interaction,
            modifier = Modifier
                .fillMaxWidth()
                .then(
                    if (multiline) Modifier.heightIn(min = Dimens.field, max = 240.dp)
                    else Modifier.height(Dimens.field)
                )
                .drawWithContent {
                    drawContent()
                    if (focused && !error) {
                        drawRoundRect(
                            color = accentSoft,
                            cornerRadius = CornerRadius(fieldRadius.toPx() + 3.dp.toPx()),
                            style = Stroke(6.dp.toPx()),
                        )
                    }
                }
                .border(1.dp, border, shape)
                .background(Tokens.surface, shape)
                .padding(horizontal = 16.dp, vertical = if (multiline) 14.dp else 0.dp),
            decorationBox = { inner ->
                Box(contentAlignment = if (multiline) Alignment.TopStart else Alignment.CenterStart) {
                    if (value.isEmpty() && placeholder.isNotEmpty()) {
                        Text(placeholder, color = Tokens.muted, fontSize = 14.sp, fontFamily = FontFamily.Monospace)
                    }
                    inner()
                }
            },
        )
    }
}

@Composable
fun Chip(text: String, warn: Boolean = false, ok: Boolean = false) {
    val bg = when {
        ok -> Tokens.accentQuiet
        warn -> Tokens.warnQuiet
        else -> Tokens.raised
    }
    val fg = when {
        ok -> Tokens.accentText
        warn -> Tokens.warnText
        else -> Tokens.muted
    }
    Box(
        Modifier
            .height(28.dp)
            .clip(RoundedCornerShape(999.dp))
            .background(bg)
            .padding(horizontal = 11.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = fg, fontFamily = FontFamily.Monospace, fontSize = 12.sp)
    }
}

@Composable
fun SwitchTrack(on: Boolean) {
    Box(
        Modifier
            .width(46.dp)
            .height(28.dp)
            .clip(RoundedCornerShape(999.dp))
            .background(if (on) Tokens.accent else Tokens.raised)
            .border(1.dp, if (on) Tokens.accent else Tokens.borderStrong, RoundedCornerShape(999.dp)),
    ) {
        Box(
            Modifier
                .padding(3.dp)
                .size(20.dp)
                .align(if (on) Alignment.CenterEnd else Alignment.CenterStart)
                .clip(CircleShape)
                .background(Tokens.surface),
        )
    }
}

@Composable
fun SkeletonBlock(broker: String) {
    val reduce = LocalReduceMotion.current
    val alpha = if (reduce) {
        0.7f
    } else {
        val t = rememberInfiniteTransition(label = "skel")
        val a by t.animateFloat(
            initialValue = 0.5f,
            targetValue = 1f,
            animationSpec = infiniteRepeatable(tween(1500), RepeatMode.Reverse),
            label = "skel-a",
        )
        a
    }
    Column(
        Modifier
            .fillMaxWidth()
            .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
            .background(Tokens.surface)
            .padding(horizontal = 16.dp, vertical = 18.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SkBar(0.42f, alpha)
        SkBar(0.72f, alpha, tall = true)
        SkBar(0.56f, alpha)
    }
    Text(
        "正在连接 $broker",
        color = Tokens.muted,
        fontSize = 12.sp,
        modifier = Modifier.fillMaxWidth().padding(top = 22.dp),
        textAlign = androidx.compose.ui.text.style.TextAlign.Center,
    )
}

@Composable
private fun SkBar(frac: Float, alpha: Float, tall: Boolean = false) {
    Box(
        Modifier
            .fillMaxWidth(frac)
            .height(if (tall) 26.dp else 12.dp)
            .clip(RoundedCornerShape(6.dp))
            .background(Tokens.raised.copy(alpha = alpha)),
    )
}

@Composable
fun OfflineBanner(lastSync: String, onRetry: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .background(Tokens.warnQuiet)
            .border(1.dp, Tokens.warn.copy(alpha = 0.34f), RoundedCornerShape(14.dp))
            .padding(horizontal = 14.dp, vertical = 13.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(UnlockerIcons.Warn, contentDescription = null, tint = Tokens.warnText, modifier = Modifier.size(15.dp))
        Column(Modifier.weight(1f).padding(horizontal = 11.dp)) {
            Text("连接中断", color = Tokens.warnText, fontSize = 13.sp, fontWeight = W650, lineHeight = 18.sp)
            Text(
                if (lastSync.isBlank()) "正在自动重试" else "最后一次同步 $lastSync · 正在自动重试",
                color = Tokens.warnText,
                fontSize = 12.sp,
                lineHeight = 18.sp,
            )
        }
        Box(
            Modifier
                .height(44.dp)
                .clip(RoundedCornerShape(999.dp))
                .border(1.dp, Tokens.warn.copy(alpha = 0.46f), RoundedCornerShape(999.dp))
                .clickable(role = Role.Button, onClick = onRetry)
                .padding(horizontal = 14.dp)
                .focusRing(22.dp),
            contentAlignment = Alignment.Center,
        ) {
            Text("重试", color = Tokens.warnText, fontSize = 13.sp, fontWeight = W650)
        }
    }
}

@Composable
fun ToastChip(text: String) {
    Box(
        Modifier
            .clip(RoundedCornerShape(999.dp))
            .background(Tokens.raised)
            .border(1.dp, Tokens.border, RoundedCornerShape(999.dp))
            .padding(horizontal = 18.dp, vertical = 11.dp),
    ) {
        Text(text, color = Tokens.fg, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun HairList(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Column(modifier.fillMaxWidth().border(width = 0.dp, color = Color.Transparent)) {
        Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
        content()
    }
}

@Composable
fun ListRow(
    title: String,
    subtitle: String? = null,
    last: Boolean = false,
    onClick: () -> Unit,
    trailing: @Composable (() -> Unit)? = { Icon(UnlockerIcons.Chevron, null, tint = Tokens.dim, modifier = Modifier.size(15.dp)) },
) {
    Row(
        Modifier
            .fillMaxWidth()
            .height(Dimens.listRow)
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { role = Role.Button }
            .padding(vertical = 12.dp)
            .focusRing(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                title,
                color = Tokens.fg,
                fontFamily = FontFamily.Monospace,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                letterSpacing = (-0.32).sp,
            )
            if (subtitle != null) {
                Text(
                    subtitle,
                    color = Tokens.muted,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                    letterSpacing = 1.8.sp,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
        trailing?.invoke()
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

@Composable
fun SettingRow(
    label: String,
    value: String? = null,
    last: Boolean = false,
    onClick: (() -> Unit)? = null,
    trailing: @Composable (() -> Unit)? = null,
) {
    val clickable = onClick != null
    Row(
        Modifier
            .fillMaxWidth()
            .height(Dimens.settingRow)
            .then(if (clickable) Modifier.clickable(role = Role.Button, onClick = onClick!!) else Modifier)
            .padding(vertical = 12.dp)
            .focusRing(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = Tokens.fg, fontSize = 15.sp, modifier = Modifier.padding(end = 14.dp))
        if (value != null) {
            Text(
                value,
                color = Tokens.muted,
                fontFamily = FontFamily.Monospace,
                fontSize = 13.sp,
                modifier = Modifier.weight(1f),
                textAlign = androidx.compose.ui.text.style.TextAlign.Right,
            )
        } else {
            Spacer(Modifier.weight(1f))
        }
        trailing?.invoke()
        if (clickable && trailing == null) {
            Icon(UnlockerIcons.Chevron, null, tint = Tokens.dim, modifier = Modifier.size(15.dp))
        }
    }
    if (!last) Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

@Composable
fun PillNav(active: Screen, pendingCount: Int, onTab: (Screen) -> Unit) {
    Row(
        Modifier
            .clip(RoundedCornerShape(999.dp))
            .background(Tokens.surface.copy(alpha = 0.92f))
            .border(1.dp, Tokens.border, RoundedCornerShape(999.dp))
            .padding(7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        PillItem("条目", UnlockerIcons.Key, active == Screen.Vault, pending = 0) { onTab(Screen.Vault) }
        PillItem("待批准", UnlockerIcons.Bell, active == Screen.Pending, pending = pendingCount) { onTab(Screen.Pending) }
        PillItem("设置", UnlockerIcons.Gear, active == Screen.Settings, pending = 0) { onTab(Screen.Settings) }
    }
}

@Composable
private fun PillItem(label: String, icon: ImageVector, on: Boolean, pending: Int, onClick: () -> Unit) {
    Box {
        Row(
            Modifier
                .height(Dimens.touch)
                .clip(RoundedCornerShape(999.dp))
                .background(if (on) Tokens.raised else Color.Transparent)
                .clickable(role = Role.Button, onClick = onClick)
                .padding(horizontal = 15.dp)
                .focusRing(24.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(7.dp),
        ) {
            Icon(icon, contentDescription = null, tint = if (on) Tokens.fg else Tokens.muted, modifier = Modifier.size(15.dp))
            Text(label, color = if (on) Tokens.fg else Tokens.muted, fontSize = 12.sp, fontWeight = W650, letterSpacing = 0.12.sp)
        }
        if (pending > 0) {
            Box(
                Modifier
                    .align(Alignment.TopEnd)
                    .padding(top = 2.dp, end = 8.dp)
                    .height(16.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .background(Tokens.warn)
                    .border(2.dp, Tokens.surface, RoundedCornerShape(999.dp))
                    .padding(horizontal = 4.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    pending.toString(),
                    color = Tokens.onWarn,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                )
            }
        }
    }
}

@Composable
fun SheetScrim(onDismiss: () -> Unit, content: @Composable () -> Unit) {
    Box(Modifier.fillMaxSize().semantics { contentDescription = "弹层" }) {
        Box(
            Modifier
                .matchParentSize()
                .background(Tokens.bg.copy(alpha = 0.68f))
                .clickable(role = Role.Button, onClick = onDismiss),
        )
        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .heightIn(max = LocalConfiguration.current.screenHeightDp.dp * 0.78f)
                .clip(RoundedCornerShape(topStart = Dimens.rSheet, topEnd = Dimens.rSheet))
                .background(Tokens.surface)
                .verticalScroll(rememberScrollState())
                .padding(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 30.dp),
        ) {
            Box(
                Modifier
                    .align(Alignment.CenterHorizontally)
                    .width(34.dp)
                    .height(4.dp)
                    .clip(RoundedCornerShape(999.dp))
                    .background(Tokens.borderStrong),
            )
            Spacer(Modifier.height(16.dp))
            content()
        }
    }
}

/** .req-cmd — 深色命令块：待批准请求卡与记录详情共用，展示近似还原的那条 CLI。 */
@Composable
fun CommandBlock(text: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(Tokens.fg)
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Text("❯", color = Tokens.ok, fontFamily = FontFamily.Monospace, fontSize = 12.sp)
        Text(
            text,
            color = Tokens.surface,
            fontFamily = FontFamily.Monospace,
            fontSize = 12.sp,
            lineHeight = 18.sp,
            modifier = Modifier.padding(start = 8.dp),
        )
    }
}

/** 「密码 / 备注」二选一胶囊；条目详情与批准页共用。值取 VaultField 里的常量。 */
@Composable
fun FieldToggle(
    selected: String,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FieldPill("密码", selected == VaultField.PASSWORD) { onSelect(VaultField.PASSWORD) }
        FieldPill("备注", selected == VaultField.NOTE) { onSelect(VaultField.NOTE) }
    }
}

@Composable
private fun FieldPill(label: String, active: Boolean, onClick: () -> Unit) {
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
