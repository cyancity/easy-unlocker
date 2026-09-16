// 常量时间比较，避免 pairing token 被逐字节试探。
export function timingSafeEqual(a: string, b: string): boolean {
  const ab = new TextEncoder().encode(a);
  const bb = new TextEncoder().encode(b);
  if (ab.length === 0 || ab.length !== bb.length) return false;
  let diff = 0;
  for (let i = 0; i < ab.length; i++) diff |= ab[i] ^ bb[i];
  return diff === 0;
}

export function bearer(header: string | null, expected: string): boolean {
  if (!expected || !header || !header.startsWith("Bearer ")) return false;
  const provided = header.slice("Bearer ".length);
  if (provided === "" || /[\s]/.test(provided)) return false;
  return timingSafeEqual(provided, expected);
}

export function bearerToken(header: string | null): string {
  if (!header || !header.startsWith("Bearer ")) return "";
  return header.slice("Bearer ".length);
}

async function hmacHex(key: string, message: string): Promise<string> {
  const cryptoKey = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(key),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const mac = await crypto.subtle.sign("HMAC", cryptoKey, new TextEncoder().encode(message));
  return [...new Uint8Array(mac)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

// 对齐 Go 的 broker/server.go decisionSignature：
//   HMAC-SHA256(key, "easy-unlocker/v1/decision/" + id + "/" + decision)
export function decisionSignature(key: string, id: string, decision: string): Promise<string> {
  return hmacHex(key, `easy-unlocker/v1/decision/${id}/${decision}`);
}
