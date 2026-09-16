package io.github.cyancity.easyunlocker.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.sin

data class Oklch(val l: Double, val c: Double, val h: Double, val a: Double = 1.0) {
    fun toColor(): Color = oklchToColor(l, c, h, a)
}

fun oklch(lPercent: Double, c: Double, h: Double, alpha: Double = 1.0): Oklch =
    Oklch(lPercent / 100.0, c, h, alpha)

fun mixOklch(from: Oklch, fromWeight: Double, to: Oklch): Oklch {
    val t = 1.0 - fromWeight
    val h1 = if (from.c < 1e-6) to.h else from.h
    val h2 = if (to.c < 1e-6) from.h else to.h
    var d = h2 - h1
    if (d > 180) d -= 360.0
    if (d < -180) d += 360.0
    val h = ((h1 + t * d) % 360.0 + 360.0) % 360.0
    return Oklch(
        l = from.l * fromWeight + to.l * t,
        c = from.c * fromWeight + to.c * t,
        h = h,
        a = from.a * fromWeight + to.a * t,
    )
}

fun mixTransparent(color: Oklch, amount: Double): Oklch = color.copy(a = color.a * amount)

private fun srgbEncode(x: Double): Double {
    val c = x.coerceIn(0.0, 1.0)
    return if (c <= 0.0031308) 12.92 * c else 1.055 * c.pow(1.0 / 2.4) - 0.055
}

private fun oklchToColor(L: Double, C: Double, hDeg: Double, alpha: Double): Color {
    val h = Math.toRadians(hDeg)
    val a = C * cos(h)
    val b = C * sin(h)
    val l_ = L + 0.3963377774 * a + 0.2158037573 * b
    val m_ = L - 0.1055613458 * a - 0.0638541728 * b
    val s_ = L - 0.0894841775 * a - 1.2914855480 * b
    val l = l_ * l_ * l_
    val m = m_ * m_ * m_
    val s = s_ * s_ * s_
    val r = +4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s
    val g = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s
    val bLin = -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s
    return Color(
        red = srgbEncode(r).toFloat(),
        green = srgbEncode(g).toFloat(),
        blue = srgbEncode(bLin).toFloat(),
        alpha = alpha.toFloat(),
    )
}

data class UnlockerTokens(
    val bg: Color,
    val surface: Color,
    val raised: Color,
    val fg: Color,
    val muted: Color,
    val border: Color,
    val accent: Color,
    val warn: Color,
    val danger: Color,
    val onAccent: Color,
    val onWarn: Color,
    val onDanger: Color,
    val borderStrong: Color,
    val ink: Color,
    val bezel: Color,
    val accentText: Color,
    val warnText: Color,
    val dangerText: Color,
    val accentFillH: Color,
    val accentQuiet: Color,
    val warnQuiet: Color,
    val dangerQuiet: Color,
    val accentSoft: Color,
    val fgSoft: Color,
    val hair: Color,
    val dim: Color,
    val shadow: Color,
    val ok: Color,
    val okText: Color,
    val okQuiet: Color,
    val isDark: Boolean,
)

fun buildTokens(dark: Boolean): UnlockerTokens {
    val bg = if (dark) oklch(29.7, 0.008, 264.0) else oklch(97.6, 0.003, 265.0)
    val surface = if (dark) oklch(32.1, 0.009, 268.0) else oklch(100.0, 0.0, 0.0)
    val raised = if (dark) oklch(34.9, 0.011, 271.0) else oklch(94.9, 0.007, 269.0)
    val fg = if (dark) oklch(96.4, 0.003, 265.0) else oklch(20.1, 0.012, 271.0)
    val muted = if (dark) oklch(73.0, 0.013, 260.0) else oklch(49.6, 0.019, 266.0)
    val border = if (dark) oklch(37.6, 0.011, 271.0) else oklch(91.8, 0.009, 265.0)
    val accent = oklch(57.7, 0.209, 274.0)
    val warn = if (dark) oklch(80.1, 0.151, 81.0) else oklch(53.3, 0.119, 66.0)
    val danger = if (dark) oklch(63.7, 0.215, 25.0) else oklch(54.5, 0.172, 24.0)
    val ok = if (dark) oklch(78.0, 0.16, 145.0) else oklch(58.0, 0.16, 145.0)
    val onAccent = oklch(100.0, 0.0, 0.0)
    val borderStrong = if (dark) oklch(58.6, 0.015, 267.0) else oklch(63.6, 0.025, 267.0)
    val ink = if (dark) oklch(17.3, 0.004, 264.0) else oklch(20.1, 0.012, 271.0)
    val onWarn = if (dark) oklch(17.3, 0.004, 264.0) else oklch(100.0, 0.0, 0.0)
    val onDanger = if (dark) oklch(17.3, 0.004, 264.0) else oklch(100.0, 0.0, 0.0)
    val mixP = if (dark) 0.55 else 0.80
    val accentSoftAmt = if (dark) 0.26 else 0.22
    val fgSoftAmt = if (dark) 0.08 else 0.07
    val hairAmt = if (dark) 0.12 else 0.10
    val dimAmt = if (dark) 0.38 else 0.40
    val shadowAmt = if (dark) 0.70 else 0.22
    return UnlockerTokens(
        bg = bg.toColor(),
        surface = surface.toColor(),
        raised = raised.toColor(),
        fg = fg.toColor(),
        muted = muted.toColor(),
        border = border.toColor(),
        accent = accent.toColor(),
        warn = warn.toColor(),
        danger = danger.toColor(),
        onAccent = onAccent.toColor(),
        onWarn = onWarn.toColor(),
        onDanger = onDanger.toColor(),
        borderStrong = borderStrong.toColor(),
        ink = ink.toColor(),
        bezel = ink.toColor(),
        accentText = mixOklch(accent, mixP, fg).toColor(),
        warnText = mixOklch(warn, mixP, fg).toColor(),
        dangerText = mixOklch(danger, mixP, fg).toColor(),
        accentFillH = mixOklch(accent, 0.78, ink).toColor(),
        accentQuiet = mixOklch(accent, 0.14, bg).toColor(),
        warnQuiet = mixOklch(warn, 0.14, bg).toColor(),
        dangerQuiet = mixOklch(danger, 0.14, bg).toColor(),
        accentSoft = mixTransparent(accent, accentSoftAmt).toColor(),
        fgSoft = mixTransparent(fg, fgSoftAmt).toColor(),
        hair = mixTransparent(fg, hairAmt).toColor(),
        dim = mixOklch(fg, dimAmt, bg).toColor(),
        shadow = mixTransparent(ink, shadowAmt).toColor(),
        ok = ok.toColor(),
        okText = mixOklch(ok, mixP, fg).toColor(),
        okQuiet = mixOklch(ok, 0.14, bg).toColor(),
        isDark = dark,
    )
}

object Dimens {
    val page = 22.dp
    val actionBottom = 14.dp
    val mainButton = 58.dp
    val field = 56.dp
    val listRow = 72.dp
    val settingRow = 64.dp
    val touch = 48.dp
    val navClearance = 96.dp
    val rCard = 16.dp
    val rNotif = 24.dp
    val rSheet = 28.dp
    val rButton = 15.dp
    val rField = 14.dp
}

val W550 = FontWeight(550)
val W650 = FontWeight(650)

val LocalTokens = staticCompositionLocalOf { buildTokens(false) }
val LocalReduceMotion = staticCompositionLocalOf { false }

val Tokens: UnlockerTokens
    @Composable get() = LocalTokens.current

private val Type = Typography(
    headlineLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = W650,
        fontSize = 34.sp,
        lineHeight = 45.sp,
        letterSpacing = 0.sp,
    ),
    headlineMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = W650,
        fontSize = 32.sp,
        lineHeight = 42.sp,
        letterSpacing = 0.sp,
    ),
    titleLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = W650,
        fontSize = 20.sp,
        lineHeight = 28.sp,
        letterSpacing = 0.sp,
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 15.sp,
        lineHeight = 22.sp,
        letterSpacing = 0.sp,
    ),
    bodyLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 25.sp,
        letterSpacing = 0.sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.Normal,
        fontSize = 13.sp,
        lineHeight = 20.sp,
        letterSpacing = 0.13.sp,
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontWeight = FontWeight.SemiBold,
        fontSize = 13.sp,
        letterSpacing = 0.13.sp,
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.Monospace,
        fontWeight = FontWeight.Normal,
        fontSize = 11.sp,
        letterSpacing = 1.1.sp,
    ),
)

@Composable
fun UnlockerTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    reduceMotion: Boolean = false,
    content: @Composable () -> Unit,
) {
    val tokens = buildTokens(darkTheme)
    val scheme = if (darkTheme) {
        darkColorScheme(
            primary = tokens.accent,
            onPrimary = tokens.onAccent,
            background = tokens.bg,
            onBackground = tokens.fg,
            surface = tokens.surface,
            onSurface = tokens.fg,
            surfaceVariant = tokens.raised,
            onSurfaceVariant = tokens.muted,
            error = tokens.danger,
            onError = tokens.onDanger,
            outline = tokens.border,
            outlineVariant = tokens.borderStrong,
            secondary = tokens.warn,
            onSecondary = tokens.onWarn,
        )
    } else {
        lightColorScheme(
            primary = tokens.accent,
            onPrimary = tokens.onAccent,
            background = tokens.bg,
            onBackground = tokens.fg,
            surface = tokens.surface,
            onSurface = tokens.fg,
            surfaceVariant = tokens.raised,
            onSurfaceVariant = tokens.muted,
            error = tokens.danger,
            onError = tokens.onDanger,
            outline = tokens.border,
            outlineVariant = tokens.borderStrong,
            secondary = tokens.warn,
            onSecondary = tokens.onWarn,
        )
    }
    CompositionLocalProvider(
        LocalTokens provides tokens,
        LocalReduceMotion provides reduceMotion,
    ) {
        MaterialTheme(colorScheme = scheme, typography = Type, content = content)
    }
}
