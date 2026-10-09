// boxpayload 的 WebCrypto 版 Seal：与 internal/boxpayload/boxpayload.go 逐字节对齐。
//   shared = X25519(eph_priv, recipient_pub)
//   key    = HKDF-SHA256(ikm=shared, salt="easy-unlocker/v2/box/", info=requestID) → 32B
//   ct     = AES-256-GCM(nonce 12B, AAD=requestID)(plaintext)
//   wire   = "v2." + b64url(eph_pub(32) | nonce(12) | ct)
// 扫码配对用它把设备令牌密封给桌面公钥；broker 只见公钥，私钥不出桌面。

const V = "v2";
const HKDF_SALT = "easy-unlocker/v2/box/";

function b64urlDecode(s: string): Uint8Array {
  const pad = s.length % 4 === 0 ? "" : "=".repeat(4 - (s.length % 4));
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/") + pad);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function b64urlEncode(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export async function sealToPublic(
  recipientPubB64: string,
  requestID: string,
  plaintext: Uint8Array,
): Promise<string> {
  const recipientPub = b64urlDecode(recipientPubB64.trim());
  if (recipientPub.length !== 32) throw new Error("invalid box public key");

  // @cloudflare/workers-types 还没收录 X25519（CF Workers 运行时是支持的），算法对象走断言。
  const x25519 = { name: "X25519" } as any;
  const ephemeral = (await crypto.subtle.generateKey(x25519, true, [
    "deriveBits",
  ])) as CryptoKeyPair;
  const recipientKey = await crypto.subtle.importKey(
    "raw",
    recipientPub,
    x25519,
    false,
    [],
  );
  const shared = await crypto.subtle.deriveBits(
    { name: "X25519", public: recipientKey } as any,
    ephemeral.privateKey,
    256,
  );

  const hkdfKey = await crypto.subtle.importKey("raw", shared, "HKDF", false, [
    "deriveBits",
  ]);
  const enc = new TextEncoder();
  const keyBytes = await crypto.subtle.deriveBits(
    {
      name: "HKDF",
      hash: "SHA-256",
      salt: enc.encode(HKDF_SALT),
      info: enc.encode(requestID),
    },
    hkdfKey,
    256,
  );
  const aesKey = await crypto.subtle.importKey("raw", keyBytes, "AES-GCM", false, [
    "encrypt",
  ]);
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ct = new Uint8Array(
    await crypto.subtle.encrypt(
      { name: "AES-GCM", iv: nonce, additionalData: enc.encode(requestID) },
      aesKey,
      plaintext,
    ),
  );

  const ephPubRaw = new Uint8Array(
    (await crypto.subtle.exportKey("raw", ephemeral.publicKey)) as ArrayBuffer,
  );
  const raw = new Uint8Array(32 + 12 + ct.length);
  raw.set(ephPubRaw, 0);
  raw.set(nonce, 32);
  raw.set(ct, 44);
  return V + "." + b64urlEncode(raw);
}
