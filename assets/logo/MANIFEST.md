# easy-unlocker — 品牌标志

## 体积纪律

这个目录只放 **SVG 源 + 确实需要的位图**。不要提交打包 zip、同一图形的多尺寸位图副本、以及已有 SVG 对应的大 PNG——它们撑大的是 `.git` 历史，事后删掉当前文件也回收不了。

需要多尺寸位图时从 SVG 现场渲染（已删的 `easy-unlocker-icon-{48,192,512,1024}.png` 就是这么来的），不要入库。

## 文件

| 文件 | 用途 |
|---|---|
| `easy-unlocker-icon.svg` / `-light.svg` | 方形图标源，深色 / 浅色。Android adaptive 前景与各密度 PNG 的渲染来源 |
| `easy-unlocker-mark.svg` | 透明标志，锁梁内部与双眼为透明负形，深浅背景通用。README 用它 |
| `easy-unlocker-logo.svg` / `-dark.svg` | 横版字标，浅底 / 深底 |
| `easy-unlocker-logo.png` / `-dark.png` | 横版位图 1600 px，给不吃 SVG 的场合（OG / 社交预览） |
| `easy-unlocker-mark-512.png` | 透明标志位图 512 px，需要 PNG 头像时用 |
| `easy-unlocker-approved-dark.jpg` | 定稿设计原图（图像模型输出，1254 px）。SVG 是按它做的几何重建，原图保留作视觉真源 |
| `easy-unlocker-preview.jpg` | 深 / 浅 / 圆形裁切三态预览，1290×540 |

配色：深色版 `#22d3ee` / `#0f172a`；浅色版 `#0891b2` / `#f1f5f9`。

`approved-dark` 与 `preview` 原为 PNG（856 KB / 216 KB），2026-09-15 转 JPEG q85（137 KB / 63 KB）给仓库减重，观感无损。同时删掉了打包 zip（3.4 MB，仓库内文件的副本）、浅色版原图（776 KB，浅色 SVG 已是其干净重建，提示词见下）与各尺寸图标 PNG 副本。目录从 6.9 MB 降到 288 KB。

## Android 集成

Adaptive icon 用独立矢量前景 + 深蓝背景，前景缩放 0.82 以在各 launcher 遮罩下保住图形；API 33 资源额外带 monochrome 层；旧式密度 PNG 一并放进 app 资源。FCM 默认通知图标与创建库页面用透明单色标志。功能按钮图标保留各自语义。

minSdk 28 ≥ 26，`mipmap-anydpi-v26` 恒定命中，`android:roundIcon` 不需要 pre-26 的 PNG 兜底。

`scripts/release.sh` 无需改动：图标随 APK 资源自动打包。

## 浅色版生成提示词（存档）

Strict color-only alternate of the approved Agent-face open lock: preserve geometry, eyes, placement and proportions; deep teal symbol on pale cool white background. Single square icon, no text or extra elements. A follow-up removed the generated checkerboard corners in favor of an opaque pale background.
