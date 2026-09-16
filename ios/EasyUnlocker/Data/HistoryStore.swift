import Foundation

/// history.json —— 批准记录，最多 200 条。iOS 版多了 via/field/took_ms/command_line
/// 四个本地字段（设计稿详情页要用）；协议格式不变。
final class HistoryStore {
    private let file: URL

    init(dir: URL) {
        file = dir.appendingPathComponent("history.json")
    }

    func load() -> [HistoryEntry] {
        guard let raw = try? Data(contentsOf: file) else { return [] }
        let decoder = JSONDecoder()
        return (try? decoder.decode([HistoryEntry].self, from: raw)) ?? []
    }

    @discardableResult
    func add(_ entry: HistoryEntry) -> [HistoryEntry] {
        let next = ([entry] + load().filter { $0.requestId != entry.requestId }).prefix(200)
        save(Array(next))
        return Array(next)
    }

    private func save(_ items: [HistoryEntry]) {
        let encoder = JSONEncoder()
        if let raw = try? encoder.encode(items) {
            try? raw.write(to: file, options: .atomic)
        }
    }
}
