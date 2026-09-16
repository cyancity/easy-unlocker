import Foundation

/// 一条凭据里可以作为「放出值」的两栏；审批时二选一。
enum VaultField: String {
    case password = "password"
    case note = "note"

    /// 默认放出非空的那一栏（密码优先）。两栏都空时也返回密码，调用方会拦下并提示。
    static func defaultFor(secret: String, note: String) -> VaultField {
        if !secret.trimmingCharacters(in: .whitespaces).isEmpty { return .password }
        if !note.trimmingCharacters(in: .whitespaces).isEmpty { return .note }
        return .password
    }
}

/// 请求方希望怎么拿到值；只用于给用户展示后果。
enum Delivery: String {
    /// 写到 target 指定的文件，内容会留在那台机器上。空串（老 CLI）等价这个。
    case file = "file"
    /// 只交给请求方进程（环境变量 / 匿名 fd），用完即焚，不落盘。
    case ephemeral = "ephemeral"
}

/// CLI 请求「列出条目名」用的保留条目名，与 cli/items.go 的 ReservedListItem 一致。
enum ReservedItem {
    static let list = "#items"
    static func isListRequest(_ item: String) -> Bool { item.trimmingCharacters(in: .whitespaces) == list }
    static func isReserved(_ name: String) -> Bool {
        name.trimmingCharacters(in: .whitespaces).caseInsensitiveCompare(list) == .orderedSame
    }
}

struct VaultItem: Identifiable, Codable, Equatable {
    let id: String
    var name: String
    var aliases: [String] = []
    /// 短值：token、密码。
    var secret: String
    /// 长文本：SSH 私钥、整段 .env、说明。可空。
    var note: String = ""
}

struct VaultPlaintext: Codable {
    var items: [VaultItem] = []
    /// 租户锚点（库 UUID），与安卓 `vault_id` 同字段名；配对时上报 Broker 归租户。
    var vaultId: String = ""

    enum CodingKeys: String, CodingKey {
        case items
        case vaultId = "vault_id"
    }

    init(items: [VaultItem] = [], vaultId: String = "") {
        self.items = items
        self.vaultId = vaultId
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        items = try c.decodeIfPresent([VaultItem].self, forKey: .items) ?? []
        vaultId = try c.decodeIfPresent(String.self, forKey: .vaultId) ?? ""
    }
}

/// 待导入条目，含明文 secret/note；只允许停留在内存，不进持久化。
struct ImportCandidate: Identifiable {
    let id: Int
    let name: String
    var aliases: [String] = []
    let secret: String
    let note: String
}

/// 勾选页展示用，不含 secret。
struct ImportOption: Identifiable {
    let id: Int
    let name: String
    let username: String
}

struct ImportMergeResult {
    var imported = 0
    var skippedName = 0
    var droppedAliases = 0
    /// 名称已存在、库里备注为空而导入项有备注：只补备注，不动其它字段。
    var backfilled = 0
}

/// 已配对的设备（不含令牌本体；令牌只在自己设备上）。
struct PairedDevice: Codable, Identifiable {
    let id: String
    let name: String
    let createdAt: String
    let lastUsedAt: String
    let expiresAt: String
    let current: Bool
}

/// 一个网关 = 一台 Broker + 这台手机在那里的设备令牌。
struct Pairing: Codable, Identifiable, Equatable {
    let id: String
    var name: String
    var url: String
    var deviceToken: String
    var createdAt: Int64 = 0
}

/// 比 URL 用的规范化：忽略大小写和结尾斜杠。
func normalizeGatewayUrl(_ url: String) -> String {
    url.trimmingCharacters(in: .whitespaces)
        .replacingOccurrences(of: "/+$", with: "", options: .regularExpression)
        .lowercased()
}

struct PendingRequest: Codable, Identifiable, Equatable {
    var id: String { requestId }
    var requestId: String = ""
    var item: String = ""
    var mode: String = ""
    var purpose: String = ""
    var ttl: Int = 0
    var target: String = ""
    var requester: String = ""
    var expiresAt: String = ""
    var state: String = ""
    var sealPublicKey: String = ""
    var delivery: String = ""
    /// sign 请求才有：要被签进证书的 SSH 公钥（authorized_keys 行）。
    var publicKey: String = ""
    /// sign 请求才有：证书 principal（要登录的用户名）。
    var sshUser: String = ""
    /// sign 请求才有：证书有效期（秒）；0 = 用默认 300。
    var certTtl: Int = 0
    var selectedItemId: String? = nil
    /// 本地选择：这次放出密码（password）还是备注（note）。不来自网关。
    var selectedField: VaultField? = nil
    /// 本地记录：这条请求第一次出现在列表里的时刻（算审批耗时用）。
    var firstSeenAt: Date = Date()

    enum CodingKeys: String, CodingKey {
        case requestId = "request_id"
        case item, mode, purpose, ttl, target, requester
        case expiresAt = "expires_at"
        case state
        case sealPublicKey = "seal_public_key"
        case delivery
        case publicKey = "public_key"
        case sshUser = "ssh_user"
        case certTtl = "cert_ttl"
    }

    init() {}

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        requestId = try c.decodeIfPresent(String.self, forKey: .requestId) ?? ""
        item = try c.decodeIfPresent(String.self, forKey: .item) ?? ""
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? ""
        purpose = try c.decodeIfPresent(String.self, forKey: .purpose) ?? ""
        ttl = try c.decodeIfPresent(Int.self, forKey: .ttl) ?? 0
        target = try c.decodeIfPresent(String.self, forKey: .target) ?? ""
        requester = try c.decodeIfPresent(String.self, forKey: .requester) ?? ""
        expiresAt = try c.decodeIfPresent(String.self, forKey: .expiresAt) ?? ""
        state = try c.decodeIfPresent(String.self, forKey: .state) ?? ""
        sealPublicKey = try c.decodeIfPresent(String.self, forKey: .sealPublicKey) ?? ""
        delivery = try c.decodeIfPresent(String.self, forKey: .delivery) ?? ""
        publicKey = try c.decodeIfPresent(String.self, forKey: .publicKey) ?? ""
        sshUser = try c.decodeIfPresent(String.self, forKey: .sshUser) ?? ""
        certTtl = try c.decodeIfPresent(Int.self, forKey: .certTtl) ?? 0
    }

    func remainingSeconds(now: Date = Date()) -> Int {
        if !expiresAt.isEmpty {
            let f = ISO8601DateFormatter()
            f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let exp = f.date(from: expiresAt) ?? ISO8601DateFormatter().date(from: expiresAt) {
                return max(0, Int(exp.timeIntervalSince(now)))
            }
        }
        return max(0, ttl)
    }

    /// 审批卡片上的命令行（近似还原当时那条 CLI；命令本体不会上传）。
    var commandLine: String {
        if ReservedItem.isListRequest(item) { return "easyGet list --refresh" }
        if mode == "sign" { return "easyGet ssh \(item)" }
        if delivery == Delivery.ephemeral.rawValue { return "easyGet env \(item) --exec" }
        return "easyGet env \(item) --write-to \(target)"
    }
}

/// 「条目名名单」的线上形状，必须与 CLI 的 ParseItemList 逐字对齐。
enum ItemListWire {
    static func encode(_ items: [VaultItem]) -> Data {
        let entries: [[String: Any]] = items.map { [
            "name": $0.name,
            "aliases": $0.aliases,
        ] }
        let obj: [String: Any] = ["v": 1, "items": entries]
        return (try? JSONSerialization.data(withJSONObject: obj)) ?? Data()
    }
}

struct HistoryEntry: Codable, Identifiable {
    var id: String { requestId }
    let requestId: String
    let at: Int64
    let item: String
    let requester: String
    let purpose: String
    let target: String
    let decision: String
    /// 批准方式：faceid / password；拒绝与超时为空。
    var via: String = ""
    /// 交付字段：密码 / 备注 / 名单 / 证书。
    var field: String = ""
    /// 从请求出现在列表到处理掉的毫秒数。
    var tookMs: Int64 = 0
    var commandLine: String = ""

    enum CodingKeys: String, CodingKey {
        case requestId = "request_id", at, item, requester, purpose, target, decision
        case via, field, tookMs = "took_ms", commandLine = "command_line"
    }

    init(requestId: String, at: Int64, item: String, requester: String, purpose: String,
         target: String, decision: String, via: String = "", field: String = "",
         tookMs: Int64 = 0, commandLine: String = "") {
        self.requestId = requestId
        self.at = at
        self.item = item
        self.requester = requester
        self.purpose = purpose
        self.target = target
        self.decision = decision
        self.via = via
        self.field = field
        self.tookMs = tookMs
        self.commandLine = commandLine
    }

    /// 老记录（安卓写的 history.json）没有 via/field/took_ms/command_line，解码给默认。
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        requestId = try c.decodeIfPresent(String.self, forKey: .requestId) ?? ""
        at = try c.decodeIfPresent(Int64.self, forKey: .at) ?? 0
        item = try c.decodeIfPresent(String.self, forKey: .item) ?? ""
        requester = try c.decodeIfPresent(String.self, forKey: .requester) ?? ""
        purpose = try c.decodeIfPresent(String.self, forKey: .purpose) ?? ""
        target = try c.decodeIfPresent(String.self, forKey: .target) ?? ""
        decision = try c.decodeIfPresent(String.self, forKey: .decision) ?? ""
        via = try c.decodeIfPresent(String.self, forKey: .via) ?? ""
        field = try c.decodeIfPresent(String.self, forKey: .field) ?? ""
        tookMs = try c.decodeIfPresent(Int64.self, forKey: .tookMs) ?? 0
        commandLine = try c.decodeIfPresent(String.self, forKey: .commandLine) ?? ""
    }
}
