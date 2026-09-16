import Foundation

/// OpenSSH wire 编码（PROTOCOL.certkeys / PROTOCOL.key）：
/// uint32/uint64 大端；string = u32 长度 + 字节本体。
struct SshWriter {
    private(set) var buf = Data()

    @discardableResult
    mutating func u32(_ value: UInt64) -> SshWriter {
        precondition(value <= 0xFFFFFFFF, "u32 out of range")
        buf.append(UInt8((value >> 24) & 0xFF))
        buf.append(UInt8((value >> 16) & 0xFF))
        buf.append(UInt8((value >> 8) & 0xFF))
        buf.append(UInt8(value & 0xFF))
        return self
    }

    @discardableResult
    mutating func u64(_ value: UInt64) -> SshWriter {
        var v = value
        for shift in stride(from: 56, through: 0, by: -8) {
            buf.append(UInt8((v >> UInt64(shift)) & 0xFF))
        }
        v = 0
        return self
    }

    @discardableResult
    mutating func raw(_ bytes: Data) -> SshWriter {
        buf.append(bytes)
        return self
    }

    @discardableResult
    mutating func str(_ bytes: Data) -> SshWriter {
        u32(UInt64(bytes.count))
        return raw(bytes)
    }

    @discardableResult
    mutating func str(_ text: String) -> SshWriter { str(Data(text.utf8)) }

    func bytes() -> Data { buf }
}

struct SshReader {
    let data: Data
    private(set) var pos = 0

    init(_ data: Data) { self.data = data }

    enum Err: LocalizedError {
        case bounds(String)
        var errorDescription: String? {
            if case .bounds(let m) = self { return m }; return "ssh wire error"
        }
    }

    mutating func byte() throws -> UInt8 {
        guard remaining() >= 1 else { throw Err.bounds("ssh wire: byte out of bounds") }
        defer { pos += 1 }
        return data[pos]
    }

    mutating func u32() throws -> UInt64 {
        guard remaining() >= 4 else { throw Err.bounds("ssh wire: u32 out of bounds") }
        var value: UInt64 = 0
        for _ in 0..<4 {
            value = (value << 8) | UInt64(data[pos])
            pos += 1
        }
        return value
    }

    mutating func u64() throws -> UInt64 {
        guard remaining() >= 8 else { throw Err.bounds("ssh wire: u64 out of bounds") }
        var value: UInt64 = 0
        for _ in 0..<8 {
            value = (value << 8) | UInt64(data[pos])
            pos += 1
        }
        return value
    }

    /// string：u32 长度 + 内容。
    mutating func bytes() throws -> Data {
        let len = Int(try u32())
        guard len <= remaining() else { throw Err.bounds("ssh wire: string out of bounds") }
        let out = data.subdata(in: pos..<(pos + len))
        pos += len
        return out
    }

    mutating func string() throws -> String {
        String(decoding: try bytes(), as: UTF8.self)
    }

    func remaining() -> Int { data.count - pos }
    func consumed() -> Int { pos }

    func expectEnd() throws {
        guard pos == data.count else { throw Err.bounds("ssh wire: trailing \(data.count - pos) bytes") }
    }
}
