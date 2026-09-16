package io.github.cyancity.easyunlocker.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.unit.dp

object UnlockerIcons {
    val Lock = stroke(
        "lock",
        "M8 10V7a4 4 0 0 1 8 0v3",
        "M5 10h14a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6a2 2 0 0 1 2-2z",
    )
    val Fingerprint = stroke(
        "fp",
        "M12 4a8 8 0 0 0-8 8v2",
        "M20 12a8 8 0 0 0-8-8",
        "M12 8a4 4 0 0 0-4 4v4",
        "M16 12a4 4 0 0 0-4-4",
        "M12 12v6",
    )
    val Key = stroke(
        "key",
        "M18.5 9a3.5 3.5 0 1 1-7 0a3.5 3.5 0 1 1 7 0",
        "M12.6 11.4 4 20v-3h3v-3h3",
    )
    val Bell = stroke("bell", "M18 16v-5a6 6 0 1 0-12 0v5l-1.5 2h15z", "M10 21h4")
    val Gear = stroke(
        "gear",
        "M4 7h16",
        "M4 12h16",
        "M4 17h16",
        "M11 7a2 2 0 1 1-4 0a2 2 0 1 1 4 0",
        "M17 12a2 2 0 1 1-4 0a2 2 0 1 1 4 0",
        "M10 17a2 2 0 1 1-4 0a2 2 0 1 1 4 0",
    )
    val Plus = stroke("plus", "M12 5v14", "M5 12h14")
    val Check = stroke("check", "m5 12.5 4.5 4.5L19 7")
    val Back = stroke("back", "m15 6-6 6 6 6")
    val Close = stroke("close", "M6 6l12 12", "M18 6 6 18")
    val Chevron = stroke("go", "m9 6 6 6-6 6")
    val Copy = stroke(
        "copy",
        "M11 9h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2z",
        "M5 15V6a2 2 0 0 1 2-2h9",
    )
    val Warn = stroke("warn", "M12 4 3 19h18z", "M12 10v4", "M12 17h.01")
    val Terminal = stroke("term", "M4 17l6-6-6-6", "M12 19h8")
    val Phone = stroke(
        "phone",
        "M9 3h6a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z",
        "M11 18h2",
    )
    val More = ImageVector.Builder("more", 24.dp, 24.dp, 24f, 24f).also { b ->
        listOf(5f, 12f, 19f).forEach { y ->
            b.addPath(
                pathData = PathParser().parsePathString("M12 ${y - 1.4}a1.4 1.4 0 1 0 0 2.8a1.4 1.4 0 1 0 0-2.8").toNodes(),
                fill = SolidColor(Color.Black),
            )
        }
    }.build()

    private fun stroke(name: String, vararg paths: String): ImageVector {
        val b = ImageVector.Builder(name, 24.dp, 24.dp, 24f, 24f)
        paths.forEach { data ->
            b.addPath(
                pathData = PathParser().parsePathString(data).toNodes(),
                fill = SolidColor(Color.Transparent),
                stroke = SolidColor(Color.Black),
                strokeLineWidth = 1.7f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
        return b.build()
    }
}
