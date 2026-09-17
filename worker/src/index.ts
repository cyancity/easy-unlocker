import { BrokerState } from "./brokerState";
import type { Outcome, PendingRecord } from "./brokerState";
import type { Env } from "./env";
import { sendFcm } from "./fcm";
import { bearer, bearerToken, decisionSignature } from "./sign";

export { BrokerState };

// 协议真源是 internal/protocol/protocol.go，这里逐字对齐：路径、字段名、状态值、HTTP 码。
// 目标：App 与 CLI 一行不改，只换 base URL。

const MAX_BODY = 64 * 1024;
const DEFAULT_MAX_TTL = 3600;
const DEFAULT_MAX_WAIT_MS = 300_000;
const DO_NAME = "default";
const ALLOWED_REQUEST_KEYS = [
  "item",
  "mode",
  "purpose",
  "ttl",
  "target",
  "requester",
  "delivery",
  "public_key",
  "ssh_user",
  "cert_ttl",
  "seal_public_key",
  "request_key",
];

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

function methodNotAllowed(): Response {
  return json(405, { status: "failed", message: "method not allowed" });
}

// 对齐 Go 的 responseHTTPStatus
function responseHTTPStatus(status: string): number {
  switch (status) {
    case "approved":
      return 200;
    case "denied":
      return 403;
    case "expired":
      return 408;
    case "failed":
      return 503;
    default:
      return 400;
  }
}

function numberVar(value: string | undefined, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

/** 对齐 Go 的 decodeJSON：限长、必须是单个 JSON 对象、拒绝未知字段。 */
async function readJson(request: Request, allowed: string[]): Promise<any | null> {
  const text = await request.text();
  if (text.length > MAX_BODY) return null;
  let body: any;
  try {
    body = JSON.parse(text);
  } catch {
    return null;
  }
  if (body === null || typeof body !== "object" || Array.isArray(body)) return null;
  for (const key of Object.keys(body)) if (!allowed.includes(key)) return null;
  return body;
}

/** 对齐 protocol.Request.Validate */
function validateRequest(body: any, maxTTL: number): string {
  const item = str(body.item).trim();
  if (item === "") return "item is required";
  if (item.length > 256) return "item is too long";
  if (body.mode !== "sign" && body.mode !== "write") return "mode is invalid";
  if (str(body.purpose).trim() === "") return "purpose is required";
  if (str(body.purpose).length > 4096) return "purpose is too long";
  if (typeof body.ttl !== "number" || !Number.isInteger(body.ttl) || body.ttl < 1) {
    return "ttl must be positive";
  }
  if (maxTTL > 0 && body.ttl > maxTTL) return "ttl is too large";
  if (str(body.target).length > 4096) return "target is too long";
  if (str(body.delivery) !== "" && str(body.delivery) !== "file" && str(body.delivery) !== "ephemeral") {
    return "delivery is invalid";
  }
  if (str(body.requester).length > 512) return "requester is too long";
  if (str(body.public_key).length > 8192) return "public_key is too long";
  if (str(body.ssh_user).length > 256) return "ssh_user is too long";
  if (str(body.seal_public_key).length > 128) return "seal_public_key is too long";
  if (str(body.request_key).length > 128) return "request_key is too long";
  if (body.cert_ttl !== undefined &&
    (typeof body.cert_ttl !== "number" || !Number.isInteger(body.cert_ttl) || body.cert_ttl < 0 || body.cert_ttl > 86400)) {
    return "cert_ttl is invalid";
  }
  // 服务端没有 CA：sign 只能由手机签发并密封回来，少了这两样请求永远批不下来。
  if (body.mode === "sign") {
    if (str(body.public_key).trim() === "") return "public_key is required for sign";
    if (str(body.seal_public_key).trim() === "") return "seal_public_key is required for sign";
  }
  return "";
}

async function callDo<T>(env: Env, path: string, body?: unknown): Promise<T> {
  const stub = env.BROKER.get(env.BROKER.idFromName(DO_NAME));
  const response = await stub.fetch(`https://broker.internal${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: { "content-type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) throw new Error(`broker state returned ${response.status}`);
  return (await response.json()) as T;
}

async function handleRequest(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const auth = request.headers.get("authorization") ?? "";
  const deviceToken = bearerToken(auth);
  let deviceId = "";
  let deviceName = "";
  let tenant = "default";
  // 配对后的设备令牌与全局 pairing token 都能发起请求：配对的目的就是让新机器能取凭据。
  if (!bearer(auth, env.EASY_UNLOCKER_PAIRING_TOKEN)) {
    if (deviceToken === "") {
      return json(401, { status: "unauthorized", message: "authorization required" });
    }
    const authz = await callDo<{ ok: boolean; id?: string; name?: string; tenant?: string }>(
      env,
      "/device-authorized",
      { deviceToken },
    );
    if (!authz.ok) {
      return json(401, { status: "unauthorized", message: "authorization required" });
    }
    deviceId = authz.id ?? "";
    deviceName = authz.name ?? "";
    // 租户从发起方令牌反查：CLI 不传租户 id，伪造不了。
    tenant = authz.tenant || "default";
  }
  const body = await readJson(request, ALLOWED_REQUEST_KEYS);
  if (body === null) {
    return json(400, { status: "invalid_request", message: "invalid request body" });
  }
  const maxTTL = numberVar(env.MAX_TTL_SECONDS, DEFAULT_MAX_TTL);
  if (validateRequest(body, maxTTL) !== "") {
    return json(400, { status: "invalid_request", message: "invalid request fields" });
  }

  const id = crypto.randomUUID().replace(/-/g, "");
  const ttl = body.ttl as number;
  const receivedAt = Date.now();
  const record = {
    id,
    item: String(body.item).trim(),
    mode: body.mode as string,
    purpose: str(body.purpose),
    ttl,
    target: str(body.target),
    requester: str(body.requester).trim() || "unknown",
    delivery: str(body.delivery).trim(),
    sealPublicKey: str(body.seal_public_key).trim(),
    publicKey: str(body.public_key).trim(),
    sshUser: str(body.ssh_user).trim(),
    certTTL: typeof body.cert_ttl === "number" ? body.cert_ttl : 0,
    deviceId,
    deviceName,
    tenant,
    requestKey: str(body.request_key).trim(),
    receivedAt,
    expiresAt: receivedAt + ttl * 1000,
    approveSig: await decisionSignature(env.EASY_UNLOCKER_DECISION_KEY, id, "approve"),
    denySig: await decisionSignature(env.EASY_UNLOCKER_DECISION_KEY, id, "deny"),
    state: "waiting" as const,
  };

  const { pushTokens, reused } = await callDo<{ pushTokens: string[]; reused?: PendingRecord }>(
    env,
    "/create",
    { record },
  );
  // 幂等续等命中：客户端断线重连，沿用原记录，不重推通知（对齐 Go 的 request_reattached）。
  const effective = reused ?? record;
  const effectiveId = effective.id;
  if (!reused) {
    // 只记数量（token 本身是秘密）：0 说明没有设备注册过推送令牌
    console.log("push targets:", pushTokens.length);
    try {
      for (const pushToken of pushTokens) {
        await sendFcm(env.EASY_UNLOCKER_FCM_CREDENTIALS, pushToken, effectiveId, new URL(request.url).origin);
      }
    } catch (error) {
      // 对齐 Go：通知发不出去就当场失败，不让人白等
      const reason = error instanceof Error ? error.message : "unknown";
      console.error("notify failed:", reason);
      await callDo(env, "/fail", { id: effectiveId, message: "notification unavailable" });
      return json(503, { status: "failed", request_id: effectiveId, message: "notification unavailable" });
    }
  }

  // 等决策：挂一个 DO 调用，决策到达时由 DO 唤醒。默认等到 TTL + 5s，封顶 MAX_WAIT_MS。
  request.signal.addEventListener(
    "abort",
    () => {
      // 断线（代理掐长连接/Ctrl-C/断网）：不取消，标 detached 进 15s 重连宽限期；
      // 超期未续由 DO alarm 清掉，手机待批准随之消失
      ctx.waitUntil(callDo(env, "/detach", { id: effectiveId }).catch(() => {}));
    },
    { once: true },
  );
  const waitMs = Math.min(ttl * 1000 + 5000, numberVar(env.MAX_WAIT_MS, DEFAULT_MAX_WAIT_MS));
  const outcome = await callDo<Outcome>(env, "/wait", { id: effectiveId, timeoutMs: waitMs });
  if (outcome.state === "timeout") {
    return json(503, {
      status: "failed",
      request_id: effectiveId,
      message: `spike：Worker 单次等待上限 ${waitMs}ms，等不到决策；用 --ttl 60 再试`,
    });
  }
  return json(responseHTTPStatus(outcome.state), {
    status: outcome.state,
    mode: outcome.state === "approved" ? effective.mode : undefined,
    payload: outcome.payload,
    request_id: effectiveId,
    message: outcome.message,
  });
}

async function handleDecision(request: Request, env: Env, id: string): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const body = await readJson(request, ["decision", "sig", "payload"]);
  if (body === null) {
    return json(400, { status: "invalid_decision", message: "invalid decision body" });
  }
  if (body.decision !== "approve" && body.decision !== "deny") {
    return json(400, { status: "invalid_decision", message: "invalid decision" });
  }
  const result = await callDo<{ ok: boolean }>(env, "/decide", {
    id,
    decision: body.decision,
    sig: str(body.sig),
    payload: str(body.payload),
    deviceToken: bearerToken(request.headers.get("authorization")),
  });
  if (!result.ok) {
    return json(403, { status: "invalid_decision", message: "invalid or already used decision" });
  }
  return json(200, { status: "accepted" });
}

async function handleForgetDevice(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const header = request.headers.get("x-admin-token");
  if (!env.EASY_UNLOCKER_ADMIN_TOKEN || !bearer(header, env.EASY_UNLOCKER_ADMIN_TOKEN)) {
    return json(401, { status: "unauthorized", message: "admin authorization required" });
  }
  const body = await readJson(request, ["device_token", "all"]);
  if (body === null) {
    return json(400, { status: "invalid_request", message: "invalid request body" });
  }
  const result = await callDo<{ removed: number }>(env, "/forget-device", {
    deviceToken: str(body.device_token).trim(),
    all: body.all === true,
  });
  return json(200, { status: "ok", removed: result.removed });
}

async function handlePending(request: Request, env: Env, admin: boolean): Promise<Response> {
  if (request.method !== "GET") return methodNotAllowed();
  if (admin) {
    const header = request.headers.get("x-admin-token");
    if (!env.EASY_UNLOCKER_ADMIN_TOKEN || !bearer(header, env.EASY_UNLOCKER_ADMIN_TOKEN)) {
      return json(401, { status: "unauthorized", message: "admin authorization required" });
    }
  } else {
    const deviceToken = bearerToken(request.headers.get("authorization"));
    const authz = await callDo<{ ok: boolean; tenant?: string }>(env, "/approver-authorized", {
      deviceToken,
    });
    if (!authz.ok) {
      return json(401, { status: "unauthorized", message: "device authorization required" });
    }
    // 设备令牌只见自己租户的 pending；admin 路径不带 tenant 全见。
    return json(200, await callDo(env, "/list", { tenant: authz.tenant ?? "" }));
  }
  return json(200, await callDo(env, "/list"));
}

async function handlePair(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  if (!bearer(request.headers.get("authorization"), env.EASY_UNLOCKER_PAIRING_TOKEN)) {
    return json(401, { status: "unauthorized", message: "authorization required" });
  }
  const body = await readJson(request, ["name", "vault_id"]).catch(() => null);
  const { token, name } = await callDo<{ token: string; name: string }>(env, "/pair", {
    name: body?.name ?? "",
    vaultId: str(body?.vault_id),
  });
  return json(200, { device_token: token, name });
}

/** 设备鉴权：和 /v1/device/pending 同一套（device token 在 Authorization: Bearer，且要 approver）。 */
async function requireDevice(request: Request, env: Env): Promise<boolean> {
  const { ok } = await callDo<{ ok: boolean }>(env, "/approver-authorized", {
    deviceToken: bearerToken(request.headers.get("authorization")),
  });
  return ok;
}

/** 已授权设备生成一次性配对码（App 里显示，新机器用它换设备令牌）。 */
async function handlePairCode(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  if (!(await requireDevice(request, env))) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  const result = await callDo<{ code: string; expiresAt: number }>(env, "/pair-code", {
    deviceToken: bearerToken(request.headers.get("authorization")),
  });
  if (!result.code) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { code: result.code, expires_at: new Date(result.expiresAt).toISOString() });
}

/** 新机器用配对码换设备令牌：无鉴权，码本身就是凭证（一次性、10 分钟）。 */
async function handleClaim(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const body = await readJson(request, ["code", "name"]).catch(() => null);
  const code = str(body?.code).trim();
  if (code === "") {
    return json(400, { status: "invalid_request", message: "code required" });
  }
  const result = await callDo<
    { token: string; name: string; expiresAt: number } | { error: string }
  >(env, "/claim", { code, name: str(body?.name) });
  if ("error" in result) {
    return json(403, { status: "invalid_claim", message: "invalid or expired code" });
  }
  return json(200, {
    device_token: result.token,
    name: result.name,
    expires_at: new Date(result.expiresAt).toISOString(),
  });
}

/** 已配对设备列表（不给令牌本体）。 */
async function handleDevices(request: Request, env: Env): Promise<Response> {
  if (request.method !== "GET") return methodNotAllowed();
  const deviceToken = bearerToken(request.headers.get("authorization"));
  const result = await callDo<{ unauthorized?: boolean; devices?: unknown[] }>(env, "/devices", {
    deviceToken,
  });
  if (result.unauthorized) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { devices: result.devices ?? [] });
}

/** 撤销某个设备（不能撤销自己）。 */
async function handleRevoke(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const body = await readJson(request, ["id"]).catch(() => null);
  const id = str(body?.id).trim();
  if (id === "") {
    return json(400, { status: "invalid_request", message: "id required" });
  }
  const result = await callDo<{ unauthorized?: boolean; removed?: number }>(env, "/revoke", {
    deviceToken: bearerToken(request.headers.get("authorization")),
    id,
  });
  if (result.unauthorized) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { status: "ok", removed: result.removed ?? 0 });
}

/** 给某个设备改名（同租户内）。 */
async function handleRename(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const body = await readJson(request, ["id", "name"]).catch(() => null);
  const id = str(body?.id).trim();
  const name = str(body?.name).trim();
  if (id === "" || name === "") {
    return json(400, { status: "invalid_request", message: "id and name required" });
  }
  const result = await callDo<{ unauthorized?: boolean; renamed?: number }>(env, "/rename", {
    deviceToken: bearerToken(request.headers.get("authorization")),
    id,
    name,
  });
  if (result.unauthorized) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { status: "ok", renamed: result.renamed ?? 0 });
}

/** 给自己这台续期 180 天。 */
async function handleRenew(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const result = await callDo<{ unauthorized?: boolean; expiresAt?: number }>(env, "/renew", {
    deviceToken: bearerToken(request.headers.get("authorization")),
  });
  if (result.unauthorized || !result.expiresAt) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { status: "ok", expires_at: new Date(result.expiresAt).toISOString() });
}

async function handlePushToken(request: Request, env: Env): Promise<Response> {
  if (request.method !== "POST") return methodNotAllowed();
  const body = await readJson(request, ["token"]);
  if (body === null || str(body.token).trim() === "") {
    return json(400, { status: "invalid_request", message: "push token required" });
  }
  const result = await callDo<{ ok: boolean }>(env, "/push-token", {
    deviceToken: bearerToken(request.headers.get("authorization")),
    token: str(body.token).trim(),
  });
  if (!result.ok) {
    return json(401, { status: "unauthorized", message: "device authorization required" });
  }
  return json(200, { status: "ok" });
}

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const path = new URL(request.url).pathname;
    try {
      if (path === "/healthz") {
        if (request.method !== "GET") return methodNotAllowed();
        return json(200, { status: "ok" });
      }
      if (path === "/v1/version") {
        if (request.method !== "GET") return methodNotAllowed();
        return json(200, { version: env.CLI_VERSION ?? "" });
      }
      if (path === "/v1/request") return await handleRequest(request, env, ctx);
      if (path === "/v1/admin/pending") return await handlePending(request, env, true);
      if (path === "/v1/admin/forget-device") return await handleForgetDevice(request, env);
      if (path === "/v1/device/pending") return await handlePending(request, env, false);
      if (path === "/v1/device/pair") return await handlePair(request, env);
      if (path === "/v1/device/pair-code") return await handlePairCode(request, env);
      if (path === "/v1/device/devices") return await handleDevices(request, env);
      if (path === "/v1/device/revoke") return await handleRevoke(request, env);
      if (path === "/v1/device/rename") return await handleRename(request, env);
      if (path === "/v1/device/renew") return await handleRenew(request, env);
      if (path === "/v1/pair/claim") return await handleClaim(request, env);
      if (path === "/v1/device/push-token") return await handlePushToken(request, env);
      if (path.startsWith("/v1/decision/")) {
        return await handleDecision(request, env, path.slice("/v1/decision/".length));
      }
      return json(404, { status: "failed", message: "not found" });
    } catch (error) {
      // 只记错误的形状，绝不记 body / payload
      console.error("broker error:", error instanceof Error ? error.message : "unknown");
      return json(503, { status: "failed", message: "broker error" });
    }
  },
};
