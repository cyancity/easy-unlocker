import Foundation

/// pairings.json —— 与安卓 PairingStore 同一文件形状。
final class PairingStore {
    private let file: URL

    init(dir: URL) {
        file = dir.appendingPathComponent("pairings.json")
    }

    func load() -> [Pairing] {
        guard let raw = try? Data(contentsOf: file),
              let arr = try? JSONSerialization.jsonObject(with: raw) as? [[String: Any]] else {
            return []
        }
        return arr.compactMap { o in
            guard let id = o["id"] as? String, !id.isEmpty,
                  let url = o["url"] as? String, !url.isEmpty else { return nil }
            return Pairing(
                id: id,
                name: o["name"] as? String ?? "",
                url: url,
                deviceToken: o["device_token"] as? String ?? "",
                createdAt: (o["created_at"] as? NSNumber)?.int64Value ?? 0
            )
        }
    }

    func save(_ items: [Pairing]) {
        let arr: [[String: Any]] = items.map { p in
            [
                "id": p.id,
                "name": p.name,
                "url": p.url,
                "device_token": p.deviceToken,
                "created_at": p.createdAt,
            ]
        }
        if let raw = try? JSONSerialization.data(withJSONObject: arr) {
            try? raw.write(to: file, options: .atomic)
        }
    }
}
