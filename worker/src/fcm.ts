// FCM HTTP v1 发送。Go 版用 golang.org/x/oauth2/google 拿 access token；
// Workers 没有那个库，这里用 WebCrypto 自己签 RS256 JWT 换 token。
// 载荷逐字对齐 broker/fcm.go:52-70（App 的推送点击依赖 data.request_id 与 channel_id）。

const FCM_SCOPE = "https://www.googleapis.com/auth/firebase.messaging";
const DEFAULT_TOKEN_URI = "https://oauth2.googleapis.com/token";

interface ServiceAccount {
  project_id: string;
  client_email: string;
  private_key: string;
  token_uri?: string;
}

// 同一 isolate 内复用 token（1 小时有效，提前 60s 换新的）。
let cached: { token: string; expiresAt: number } | null = null;

function pemToPkcs8(pem: string): ArrayBuffer {
  const body = pem.replace(/-----[^-]+-----/g, "").replace(/\s+/g, "");
  const raw = atob(body);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes.buffer;
}

function b64urlBytes(bytes: Uint8Array): string {
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function b64urlJson(value: unknown): string {
  return b64urlBytes(new TextEncoder().encode(JSON.stringify(value)));
}

async function accessToken(sa: ServiceAccount): Promise<string> {
  if (cached && cached.expiresAt > Date.now() + 60_000) return cached.token;
  const now = Math.floor(Date.now() / 1000);
  const signingInput =
    `${b64urlJson({ alg: "RS256", typ: "JWT" })}.` +
    b64urlJson({
      iss: sa.client_email,
      scope: FCM_SCOPE,
      aud: sa.token_uri || DEFAULT_TOKEN_URI,
      iat: now,
      exp: now + 3600,
    });
  const key = await crypto.subtle.importKey(
    "pkcs8",
    pemToPkcs8(sa.private_key),
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign(
    "RSASSA-PKCS1-v1_5",
    key,
    new TextEncoder().encode(signingInput),
  );
  const assertion = `${signingInput}.${b64urlBytes(new Uint8Array(signature))}`;

  const response = await fetch(sa.token_uri || DEFAULT_TOKEN_URI, {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "urn:ietf:params:oauth:grant-type:jwt-bearer",
      assertion,
    }),
  });
  if (!response.ok) throw new Error("google token exchange failed");
  const body = (await response.json()) as { access_token?: string; expires_in?: number };
  if (!body.access_token) throw new Error("google token exchange returned no token");
  cached = {
    token: body.access_token,
    expiresAt: Date.now() + (body.expires_in ?? 3600) * 1000,
  };
  return cached.token;
}

/** 推送「有一条待批准」。凭据没配或设备没注册 token 时静默跳过（跟 Go 版一致）。 */
export async function sendFcm(
  credentialsJson: string | undefined,
  deviceToken: string,
  requestId: string,
  gateway: string,
): Promise<void> {
  if (!credentialsJson?.trim() || !deviceToken.trim()) return;
  const sa = JSON.parse(credentialsJson) as ServiceAccount;
  if (!sa.project_id || !sa.client_email || !sa.private_key) {
    throw new Error("FCM credentials missing fields");
  }
  // 载荷里只有 request_id 与网关标识：不写条目名、路径、用途（详情进 App 再看）
  const data: Record<string, string> = { request_id: requestId };
  if (gateway.trim() !== "") data.gateway = gateway.trim().replace(/\/+$/, "");
  const token = await accessToken(sa);
  const response = await fetch(
    `https://fcm.googleapis.com/v1/projects/${sa.project_id}/messages:send`,
    {
      method: "POST",
      headers: {
        authorization: `Bearer ${token}`,
        "content-type": "application/json",
      },
      body: JSON.stringify({
        message: {
          token: deviceToken,
          notification: { title: "easy-unlocker", body: "有一条待批准的请求" },
          data,
          android: {
            priority: "HIGH",
            notification: { channel_id: "unlock_v2" },
          },
        },
      }),
    },
  );
  if (response.body) await response.body.cancel();
  if (!response.ok) {
    // 只带状态码，不带 device token / body
    throw new Error(`FCM send returned ${response.status}`);
  }
}
