import Foundation

/// 与安卓 BrokerClient 同一组 REST 端点，报文字段一字不改。
final class BrokerClient {
    let brokerUrl: String
    let deviceToken: String

    init(brokerUrl: String, deviceToken: String) {
        self.brokerUrl = brokerUrl
        self.deviceToken = deviceToken
    }

    struct BrokerError: LocalizedError {
        let code: Int
        let body: String
        var errorDescription: String? { "Broker \(code) \(body)" }
    }

    func pair(pairingToken: String, name: String, vaultId: String = "") async throws -> String {
        let raw = try await request("POST", "/v1/device/pair", ["name": name, "vault_id": vaultId], bearer: pairingToken)
        return raw["device_token"] as? String ?? ""
    }

    func pending() async throws -> [PendingRequest] {
        let data = try await requestRaw("GET", "/v1/device/pending", nil, bearer: deviceToken)
        let arr = (try? JSONSerialization.jsonObject(with: data)) as? [[String: Any]] ?? []
        return arr.map { o in
            var r = PendingRequest()
            r.requestId = o["request_id"] as? String ?? ""
            r.item = o["item"] as? String ?? ""
            r.mode = o["mode"] as? String ?? ""
            r.purpose = o["purpose"] as? String ?? ""
            r.ttl = (o["ttl"] as? NSNumber)?.intValue ?? 0
            r.target = o["target"] as? String ?? ""
            r.requester = o["requester"] as? String ?? ""
            r.expiresAt = o["expires_at"] as? String ?? ""
            r.state = o["state"] as? String ?? ""
            r.sealPublicKey = o["seal_public_key"] as? String ?? ""
            r.delivery = o["delivery"] as? String ?? ""
            r.publicKey = o["public_key"] as? String ?? ""
            r.sshUser = o["ssh_user"] as? String ?? ""
            r.certTtl = (o["cert_ttl"] as? NSNumber)?.intValue ?? 0
            return r
        }
    }

    func registerPushToken(_ pushToken: String) async throws {
        if pushToken.isEmpty { return }
        _ = try await request("POST", "/v1/device/push-token", ["token": pushToken], bearer: deviceToken)
    }

    func decide(requestId: String, decision: String, payload: String?) async throws {
        var body: [String: Any] = ["decision": decision]
        if let payload, !payload.isEmpty { body["payload"] = payload }
        _ = try await request("POST", "/v1/decision/\(requestId)", body, bearer: deviceToken)
    }

    /// 生成一次性配对码（10 分钟），给新机器的 `easyGet pair` 用。
    func createPairCode() async throws -> (code: String, expiresAt: Int64) {
        let raw = try await request("POST", "/v1/device/pair-code", [:], bearer: deviceToken)
        let code = raw["code"] as? String ?? ""
        var expiresAt: Int64 = 0
        if let s = raw["expires_at"] as? String,
           let d = ISO8601DateFormatter().date(from: s) {
            expiresAt = Int64(d.timeIntervalSince1970 * 1000)
        }
        return (code, expiresAt)
    }

    /// 已配对设备列表（不含令牌本体）。
    func devices() async throws -> [PairedDevice] {
        let raw = try await request("GET", "/v1/device/devices", nil, bearer: deviceToken)
        let arr = raw["devices"] as? [[String: Any]] ?? []
        return arr.map { o in
            PairedDevice(
                id: o["id"] as? String ?? "",
                name: o["name"] as? String ?? "",
                createdAt: o["created_at"] as? String ?? "",
                lastUsedAt: o["last_used_at"] as? String ?? "",
                expiresAt: o["expires_at"] as? String ?? "",
                current: o["current"] as? Bool ?? false
            )
        }
    }

    /// 撤销一台设备（可以是当前这台 = 登出）。
    func revokeDevice(id: String) async throws {
        _ = try await request("POST", "/v1/device/revoke", ["id": id], bearer: deviceToken)
    }

    /// 给当前设备续期 180 天，返回新的过期时间。
    func renewDevice() async throws -> String {
        let raw = try await request("POST", "/v1/device/renew", [:], bearer: deviceToken)
        return raw["expires_at"] as? String ?? ""
    }

    /// 给一台设备改名（同租户内，可以是当前这台）。
    func renameDevice(id: String, name: String) async throws {
        _ = try await request("POST", "/v1/device/rename", ["id": id, "name": name], bearer: deviceToken)
    }

    // MARK: - 底层

    @discardableResult
    private func request(_ method: String, _ path: String, _ body: [String: Any]?, bearer: String) async throws -> [String: Any] {
        let data = try await requestRaw(method, path, body, bearer: bearer)
        return (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? [:]
    }

    private func requestRaw(_ method: String, _ path: String, _ body: [String: Any]?, bearer: String) async throws -> Data {
        let url = URL(string: brokerUrl.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + path)!
        var req = URLRequest(url: url, timeoutInterval: 15)
        req.httpMethod = method
        req.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
        if let body {
            req.httpBody = try JSONSerialization.data(withJSONObject: body)
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        let code = (resp as? HTTPURLResponse)?.statusCode ?? -1
        guard (200...299).contains(code) else {
            throw BrokerError(code: code, body: String(decoding: data, as: UTF8.self))
        }
        return data
    }
}
