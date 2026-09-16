import SwiftUI

/// Design tokens from docs/design/ios/screens/screen.css — keep names aligned.
enum Tokens {
    static let bg = Color(red: 1, green: 1, blue: 1)                    // #ffffff
    static let surface = Color(red: 0.961, green: 0.961, blue: 0.969)   // #f5f5f7
    static let surfaceWarm = Color(red: 0.984, green: 0.984, blue: 0.992) // #fbfbfd
    static let fg = Color(red: 0.114, green: 0.114, blue: 0.122)        // #1d1d1f
    static let fg2 = Color(red: 0.259, green: 0.259, blue: 0.271)       // #424245
    static let muted = Color(red: 0.431, green: 0.431, blue: 0.451)     // #6e6e73
    static let meta = Color(red: 0.525, green: 0.525, blue: 0.545)      // #86868b
    static let border = Color(red: 0.824, green: 0.824, blue: 0.843)    // #d2d2d7
    static let borderSoft = Color(red: 0.910, green: 0.910, blue: 0.929) // #e8e8ed
    static let accent = Color(red: 0, green: 0.443, blue: 0.890)        // #0071e3
    static let accentActive = Color(red: 0, green: 0.4, blue: 0.8)      // #0066cc
    static let success = Color(red: 0.086, green: 0.639, blue: 0.290)   // #16a34a
    static let warn = Color(red: 0.918, green: 0.702, blue: 0.031)      // #eab308
    static let danger = Color(red: 0.863, green: 0.149, blue: 0.149)    // #dc2626
}

extension Font {
    static func mono(_ size: CGFloat, weight: Font.Weight = .regular) -> Font {
        .system(size: size, weight: weight, design: .monospaced)
    }
}

/// Small pill label — .chip / .chip.ok / .chip.warn / .chip.bad / .chip.info
struct Chip: View {
    enum Tone { case plain, ok, warn, bad, info }
    let text: String
    var tone: Tone = .plain

    var body: some View {
        Text(text)
            .font(.system(size: 12, weight: .medium))
            .foregroundStyle(fg)
            .padding(.horizontal, 10)
            .padding(.vertical, 3)
            .frame(minHeight: 26)
            .background(bg)
            .overlay(Capsule().stroke(lineColor, lineWidth: 1))
            .clipShape(Capsule())
    }

    private var fg: Color {
        switch tone {
        case .plain: return Tokens.fg2
        case .ok: return Tokens.success
        case .warn: return Color(red: 0.55, green: 0.42, blue: 0.02)
        case .bad: return Tokens.danger
        case .info: return Tokens.accentActive
        }
    }
    private var lineColor: Color {
        switch tone {
        case .plain: return Tokens.border
        case .ok: return Tokens.success.opacity(0.35)
        case .warn: return Tokens.warn.opacity(0.4)
        case .bad: return Tokens.danger.opacity(0.3)
        case .info: return Tokens.accent.opacity(0.28)
        }
    }
    private var bg: Color {
        switch tone {
        case .plain: return Tokens.bg
        case .ok: return Tokens.success.opacity(0.08)
        case .warn: return Tokens.warn.opacity(0.10)
        case .bad: return Tokens.danger.opacity(0.07)
        case .info: return Tokens.accent.opacity(0.07)
        }
    }
}

/// .btn-pill — full-width capsule primary button.
struct PillButtonStyle: ButtonStyle {
    var disabled = false
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 17, weight: .semibold))
            .tracking(-0.17)
            .foregroundStyle(.white)
            .frame(maxWidth: .infinity, minHeight: 50)
            .padding(.horizontal, 20)
            .background(disabled ? Tokens.accent.opacity(0.38) : Tokens.accent)
            .clipShape(Capsule())
            .scaleEffect(configuration.isPressed && !disabled ? 0.985 : 1)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}

/// .btn-tinted — same silhouette, light tinted fill.
struct TintedButtonStyle: ButtonStyle {
    var danger = false
    var disabled = false
    var size: Double = 17
    var minHeight: Double = 46
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: size, weight: .semibold))
            .tracking(-0.17)
            .foregroundStyle(danger ? Tokens.danger : Tokens.accentActive)
            .frame(maxWidth: .infinity, minHeight: minHeight)
            .padding(.horizontal, 14)
            .background((danger ? Tokens.danger : Tokens.accent).opacity(danger ? 0.09 : 0.10))
            .clipShape(Capsule())
            .opacity(disabled ? 0.45 : 1)
            .scaleEffect(configuration.isPressed && !disabled ? 0.985 : 1)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}

/// .btn-quiet — plain text button.
struct QuietButtonStyle: ButtonStyle {
    var danger = false
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 17, weight: .medium))
            .foregroundStyle(danger ? Tokens.danger : Tokens.accent)
            .frame(maxWidth: .infinity, minHeight: 44)
            .background(configuration.isPressed ? Tokens.accent.opacity(0.08) : .clear)
            .clipShape(Capsule())
    }
}

/// .igroup — white rounded card holding inset-grouped rows.
struct IGroup<Content: View>: View {
    @ViewBuilder var content: Content
    var body: some View {
        VStack(spacing: 0) { content }
            .background(Tokens.bg)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .padding(.horizontal, 16)
    }
}

/// .irow separator — 1px line inset 16 from the left, like .irow + .irow::before.
struct RowSeparator: View {
    var body: some View {
        Rectangle()
            .fill(Tokens.borderSoft)
            .frame(height: 1)
            .padding(.leading, 16)
    }
}

/// .kv-label — small uppercase row label above a card.
struct KVLabel<Trailing: View>: View {
    let title: String
    @ViewBuilder var trailing: Trailing
    init(_ title: String, @ViewBuilder trailing: () -> Trailing = { EmptyView() }) {
        self.title = title
        self.trailing = trailing()
    }
    var body: some View {
        HStack {
            Text(title)
                .font(.system(size: 12))
                .tracking(0.6)
                .textCase(.uppercase)
                .foregroundStyle(Tokens.muted)
            Spacer()
            trailing
        }
        .padding(.horizontal, 20)
        .padding(.top, 20)
        .padding(.bottom, 6)
    }
}

/// .kv-card — white bordered value card.
struct KVCard<Content: View>: View {
    var accent = false
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 0) { content }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 16)
            .padding(.vertical, 14)
            .background(accent ? Tokens.accent.opacity(0.07) : Tokens.bg)
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .stroke(accent ? Tokens.accent.opacity(0.22) : Tokens.borderSoft, lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .padding(.horizontal, 16)
    }
}

/// .notice — info/warn banner.
struct Notice: View {
    enum Tone { case info, warn }
    let tone: Tone
    let title: String
    let bodyText: String
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title).font(.system(size: 14, weight: .semibold))
            Text(bodyText).font(.system(size: 14)).lineSpacing(2)
        }
        .foregroundStyle(tone == .info ? Tokens.accentActive : Color(red: 0.45, green: 0.34, blue: 0.05))
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(tone == .info ? Tokens.accent.opacity(0.09) : Tokens.warn.opacity(0.14))
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, 16)
        .padding(.top, 12)
    }
}

/// .foot-note — centered footer note.
struct FootNote: View {
    let text: String
    var mono = false
    var alignLeft = false
    var body: some View {
        Text(text)
            .font(mono ? .mono(12) : .system(size: 12))
            .foregroundStyle(Tokens.muted)
            .lineSpacing(2)
            .multilineTextAlignment(alignLeft ? .leading : .center)
            .frame(maxWidth: .infinity, alignment: alignLeft ? .leading : .center)
            .padding(.horizontal, alignLeft ? 20 : 32)
            .padding(.top, 20)
            .padding(.bottom, 8)
    }
}
