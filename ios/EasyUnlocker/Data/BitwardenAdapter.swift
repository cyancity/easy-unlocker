import Foundation

/// Bitwarden 未加密导出（.json）适配器——与安卓 BitwardenAdapter 同语义。
///
/// 条目名 **或** `login.username` 都能跟 easyGet 的请求名对上：
/// 密码取 `login.password`、备注取 `notes`，导入后 username 落到别名上，
/// `matchOrNull` 不用改就能「对得上就返回值」。
///
/// 认两种类型：`type == 1`（登录，密码栏 + notes）与 `type == 2`（安全笔记，只有 notes）。
/// SSH 私钥这类通常以安全笔记存放。`login.totp` / `fields` / `uris` 可能含明文秘密，
/// 一律不读，也不打日志。
enum BitwardenAdapter {
    /// 选文件时读入的上限；BW 导出通常远小于这个数。
    static let maxBytes = 16 * 1024 * 1024

    private static let typeLogin = 1
    private static let typeSecureNote = 2

    struct Result {
        var entries: [ImportCandidate] = []
        var skippedOtherType = 0
        var skippedNoSecret = 0
        var skippedNoName = 0
        var skippedDuplicate = 0
        /// 文件里登录 + 安全笔记条目总数（含被跳过的）。
        var candidateCount: Int { entries.count + skippedNoSecret + skippedNoName + skippedDuplicate }
    }

    static func parse(_ raw: String) throws -> Result {
        guard let data = raw.data(using: .utf8),
              let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            throw EuError.badInput("不是 JSON 文件。请选 Bitwarden 导出的 .json。")
        }
        if (root["encrypted"] as? Bool) == true {
            throw EuError.badInput("这是加密导出。请在 Bitwarden 里选「.json（未加密）」重新导一次。")
        }
        guard let items = root["items"] as? [[String: Any]] else {
            throw EuError.badInput("不是 Bitwarden 导出文件：里面没有 items。")
        }

        var r = Result()
        // 只按名称去重（BW 允许同名条目，这里只留第一条）。用户名的歧义留给
        // importEntries 一起判：同名用户名出现在多条上时整批都不记别名。
        var claimedNames = Set<String>()

        for obj in items {
            let type = (obj["type"] as? NSNumber)?.intValue ?? 0
            guard type == typeLogin || type == typeSecureNote else {
                r.skippedOtherType += 1
                continue
            }
            let login = type == typeLogin ? obj["login"] as? [String: Any] : nil
            let username = str(login, "username").trimmingCharacters(in: .whitespaces)
            let name = str(obj, "name").trimmingCharacters(in: .whitespaces).isEmpty
                ? username : str(obj, "name").trimmingCharacters(in: .whitespaces)
            if name.isEmpty {
                r.skippedNoName += 1
                continue
            }
            // 密码/备注原样保留：前后空格可能是密钥的一部分，只判空白。
            let password = str(login, "password")
            let note = str(obj, "notes")
            if password.trimmingCharacters(in: .whitespaces).isEmpty
                && note.trimmingCharacters(in: .whitespaces).isEmpty {
                r.skippedNoSecret += 1
                continue
            }
            let key = name.lowercased()
            if claimedNames.contains(key) {
                r.skippedDuplicate += 1
                continue
            }
            let aliases = [username].filter { !$0.isEmpty && $0.lowercased() != key }
            r.entries.append(ImportCandidate(
                id: r.entries.count, name: name, aliases: aliases,
                secret: password, note: note
            ))
            claimedNames.insert(key)
        }
        return r
    }

    /// 取字符串；缺字段和 JSON null 都当空串（NSNull 会变 "null" 字面量）。
    private static func str(_ obj: [String: Any]?, _ key: String) -> String {
        guard let v = obj?[key], !(v is NSNull) else { return "" }
        return v as? String ?? ""
    }
}
