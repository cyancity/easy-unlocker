import SwiftUI

/// .ttl-ring — SVG 环形倒计时：<30s 整环变红，归零由调用方处理。
struct TtlRing: View {
    let remain: Int
    let total: Int

    var body: some View {
        ZStack {
            Circle()
                .stroke(Tokens.borderSoft, lineWidth: 3.5)
            Circle()
                .trim(from: 0, to: pct)
                .stroke(hot ? Tokens.danger : Tokens.success,
                        style: StrokeStyle(lineWidth: 3.5, lineCap: .round))
                .rotationEffect(.degrees(-90))
                .animation(.easeOut(duration: 0.22), value: hot)
            Text(fmt(remain))
                .font(.mono(10.5, weight: .semibold))
                .foregroundStyle(hot ? Tokens.danger : Tokens.fg2)
        }
        .frame(width: 46, height: 46)
        .accessibilityLabel("剩余时间")
    }

    private var hot: Bool { remain <= 30 }
    private var pct: CGFloat {
        CGFloat(remain) / CGFloat(max(1, total))
    }
    private func fmt(_ s: Int) -> String {
        let v = max(0, s)
        return "\(v / 60):\(String(format: "%02d", v % 60))"
    }
}

/// .req-head 左侧终端图标。
struct ReqDeviceIcon: View {
    var body: some View {
        RoundedRectangle(cornerRadius: 12)
            .fill(Tokens.surface)
            .frame(width: 44, height: 44)
            .overlay {
                Image(systemName: "apple.terminal")
                    .font(.system(size: 21, weight: .regular))
                    .foregroundStyle(Tokens.fg2)
            }
    }
}

/// .req-cmd — 深色命令块。
struct CommandBlock: View {
    let text: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text("❯")
                .foregroundStyle(Tokens.success)
            Text(text)
                .foregroundStyle(Tokens.surface)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .font(.mono(12))
        .lineSpacing(3)
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Tokens.fg)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
}

/// .req-field — 卡内字段（小写标签 + 内容）。
struct ReqField<Content: View>: View {
    let label: String
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(label)
                .font(.system(size: 11))
                .tracking(0.55)
                .textCase(.uppercase)
                .foregroundStyle(Tokens.meta)
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// .req-card — 请求快照卡（待批准 + 记录详情共用）。
struct RequestCard<Content: View>: View {
    let requester: String
    var subtitle = "正在请求一条凭据"
    @ViewBuilder var trailing: () -> AnyView
    @ViewBuilder var content: Content

    init(requester: String, subtitle: String = "正在请求一条凭据",
         @ViewBuilder trailing: @escaping () -> some View = { EmptyView() },
         @ViewBuilder content: () -> Content) {
        self.requester = requester
        self.subtitle = subtitle
        self.trailing = { AnyView(trailing()) }
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 12) {
                ReqDeviceIcon()
                VStack(alignment: .leading, spacing: 1) {
                    Text(requester)
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundStyle(Tokens.fg)
                    Text(subtitle)
                        .font(.system(size: 14))
                        .foregroundStyle(Tokens.muted)
                }
                Spacer(minLength: 0)
                trailing()
            }
            content
                .padding(.top, 18)
        }
        .padding(16)
        .background(Tokens.bg)
        .clipShape(RoundedRectangle(cornerRadius: 18))
        .padding(.horizontal, 16)
        .padding(.top, 10)
    }
}

/// .copybtn — 复制按钮，成功后短暂变绿；剪贴板 30 秒后自动清空。
struct CopyButton: View {
    let text: String
    @Binding var copied: Bool
    @State private var gen = 0

    var body: some View {
        Button {
            UIPasteboard.general.string = text
            copied = true
            gen += 1
            let g = gen
            Task {
                try? await Task.sleep(nanoseconds: 1_600_000_000)
                if gen == g { await MainActor.run { copied = false } }
            }
            // 30 秒后清空剪贴板（与安卓一致）
            let pasteGen = gen
            Task {
                try? await Task.sleep(nanoseconds: 30_000_000_000)
                if pasteGen == gen {
                    await MainActor.run { UIPasteboard.general.string = "" }
                }
            }
        } label: {
            HStack(spacing: 4) {
                Image(systemName: "doc.on.doc")
                    .font(.system(size: 11, weight: .semibold))
                Text(copied ? "已复制" : "复制")
                    .font(.system(size: 12, weight: .semibold))
            }
            .foregroundStyle(copied ? Tokens.success : Tokens.accent)
            .padding(.horizontal, 12)
            .padding(.vertical, 4)
            .frame(minHeight: 32)
            .background(Tokens.surface)
            .overlay(Capsule().stroke(copied ? Tokens.success.opacity(0.4) : Tokens.border, lineWidth: 1))
            .clipShape(Capsule())
        }
        .buttonStyle(.plain)
    }
}

/// 强制收起键盘（sendAction resignFirstResponder 技巧）——@FocusState 管不到
/// XCUITest/系统直接聚焦的 UIKit 字段，sheet 关闭/页面切换前统一调它。
func dismissKeyboard() {
    UIApplication.shared.sendAction(
        #selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
}
