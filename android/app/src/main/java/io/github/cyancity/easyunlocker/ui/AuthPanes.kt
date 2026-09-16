package io.github.cyancity.easyunlocker.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.cyancity.easyunlocker.R
import io.github.cyancity.easyunlocker.UiState

@Composable
fun SetupPane(code: String, onCreate: () -> Unit, onImport: () -> Unit) {
    Column(Modifier.fillMaxSize()) {
        Column(
            Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = Dimens.page),
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(painterResource(R.drawable.ic_unlocker), contentDescription = null, tint = Tokens.accentText, modifier = Modifier.size(26.dp))
            Spacer(Modifier.height(22.dp))
            Text(
                "先把恢复码落到这台手机。",
                color = Tokens.fg,
                fontSize = 34.sp,
                fontWeight = W650,
                lineHeight = 45.sp,
                letterSpacing = 0.sp,
            )
            Spacer(Modifier.height(22.dp))
            Box(
                Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(Dimens.rCard))
                    .background(Tokens.surface)
                    .border(1.dp, Tokens.border, RoundedCornerShape(Dimens.rCard))
                    .padding(horizontal = 16.dp, vertical = 20.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    code,
                    color = Tokens.fg,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 16.sp,
                    textAlign = TextAlign.Center,
                )
            }
            Text(
                "已存到 Documents/easy-unlocker-recovery.txt",
                color = Tokens.muted,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(top = 9.dp),
            )
            Text(
                "换机时没有它，这份库打不开。它不上传、不联网。",
                color = Tokens.warnText,
                fontSize = 12.sp,
                lineHeight = 18.sp,
                modifier = Modifier.padding(top = 9.dp),
            )
        }
        ActionsColumn {
            PrimaryButton("指纹确认并创建", onClick = onCreate)
            QuietButton("导入已有备份", onClick = onImport)
        }
    }
}

@Composable
fun UnlockPane(
    state: UiState,
    skipVisual: Boolean,
    onFingerprint: () -> Unit,
    onUnlock: (String) -> Unit,
    onUnlockPassword: (String) -> Unit,
) {
    val auto = state.canFingerprint && (state.pending.isNotEmpty() || state.fromNotification)
    var asked by remember { mutableStateOf(false) }
    var showRecovery by remember { mutableStateOf(false) }
    var code by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    LaunchedEffect(auto) {
        if (auto && !asked) {
            asked = true
            onFingerprint()
        }
    }
    if (skipVisual) {
        Box(Modifier.fillMaxSize().background(Tokens.bg))
        return
    }
    val pending = state.pending
    Column(Modifier.fillMaxSize()) {
        Column(
            Modifier.weight(1f).fillMaxWidth().padding(horizontal = Dimens.page),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Box(contentAlignment = Alignment.Center) {
                Box(Modifier.size(148.dp).clip(CircleShape).background(Tokens.accentQuiet))
                Box(
                    Modifier
                        .size(132.dp)
                        .clip(CircleShape)
                        .border(1.5.dp, Tokens.accent, CircleShape)
                        .clickable(role = Role.Button, onClick = onFingerprint)
                        .semantics { contentDescription = "指纹解锁" }
                        .focusRing(66.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(UnlockerIcons.Fingerprint, contentDescription = null, tint = Tokens.accentText, modifier = Modifier.size(52.dp))
                }
            }
            Spacer(Modifier.height(10.dp))
            Text(
                if (pending.isNotEmpty()) "有 ${pending.size} 条请求在等你" else "库已锁上",
                color = Tokens.fg,
                fontSize = 34.sp,
                fontWeight = W650,
                lineHeight = 45.sp,
                letterSpacing = 0.sp,
                textAlign = TextAlign.Center,
            )
            if (pending.isNotEmpty()) {
                Text(
                    pending.joinToString(" · ") { it.item },
                    color = Tokens.muted,
                    fontFamily = FontFamily.Monospace,
                    fontSize = 13.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
        }
        ActionsColumn {
            PrimaryButton("指纹解锁", onClick = onFingerprint)
            if (state.hasPassword) {
                Field(password, { password = it }, "解锁密码", placeholder = "输入解锁密码", secret = true)
                QuietButton("密码解锁") { onUnlockPassword(password) }
            }
            if (showRecovery) {
                Field(code, { code = it }, "恢复码", placeholder = "粘贴恢复码")
                QuietButton("用恢复码解锁") { onUnlock(code) }
            } else {
                DeclineButton("用恢复码") { showRecovery = true }
            }
        }
    }
}
