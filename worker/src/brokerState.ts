import { DurableObject } from "cloudflare:workers";
import type { Env } from "./env";
import { timingSafeEqual } from "./sign";

// 单个 DO 实例（名字固定 "default"）里保存全部状态，等价于 Go 版那把 sync.Mutex 保护的两个 map，
// 换来的是强一致：/v1/device/pending 一读就能看到刚创建的请求，不会像 KV 那样读到 60s 前的旧值。
const REQ_PREFIX = "req:";
const DEV_PREFIX = "dev:";
const CODE_PREFIX = "code:";

type PendingState = "waiting" | "approved" | "denied" | "expired" | "failed";

export interface PendingRecord {
  id: string;
  item: string;
  mode: string;
  purpose: string;
  ttl: number;
  target: string;
  requester: string;
  delivery: string;
  sealPublicKey: string;
  publicKey: string;
  sshUser: string;
  certTTL: number;
  deviceId: string;
  deviceName: string;
  /** 租户 = 发起方令牌绑定的 vault_id；pairing token 直发归 "default"。 */
  tenant: string;
  /** 客户端幂等键：断线重连带同 key 续等原记录，不重复出现。 */
  requestKey?: string;
  /** 客户端断线时间：>0 时进重连宽限期，超期未续由 alarm 清掉（对齐 Go detachGrace）。 */
  detachedAt?: number;
  receivedAt: number;
  expiresAt: number;
  approveSig: string;
  denySig: string;
  state: PendingState;
  payload?: string;
  message?: string;
}

/** 设备令牌默认有效期：180 天；到期前 App 会提示续期（见 /renew）。 */
const DEVICE_TTL_MS = 180 * 24 * 60 * 60 * 1000;
/** 一次性配对码有效期：10 分钟。 */
const PAIR_CODE_TTL_MS = 10 * 60 * 1000;
/** 配对码字符集：去掉 0/O/1/I 这类看起来像的。 */
const PAIR_CODE_ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
/** lastUsedAt 的写入节流，别每个请求都写一次 DO 存储。 */
const LAST_USED_TOUCH_MS = 60 * 60 * 1000;
/** 断线宽限期：客户端连接断了 pending 再留这么久等重连（对齐 Go detachGrace）。 */
const DETACH_GRACE_MS = 15_000;

interface DeviceRecord {
  /** 对外标识，用于撤销；令牌本体只在自己设备上，不下发。 */
  id: string;
  name: string;
  role?: "approver" | "requester";
  /** 租户 = 库初始化时生成的 vault_id；老记录没有，归 "default"。 */
  tenant?: string;
  created: number;
  expiresAt: number;
  lastUsedAt?: number;
  pushToken?: string;
}

/** 老记录没有 role 字段：当 approver 处理，当 requester 会把在用的手机废掉。 */
function roleOf(device: DeviceRecord): "approver" | "requester" {
  return device.role ?? "approver";
}

const DEFAULT_TENANT = "default";

/** 老记录没有 tenant 字段：归入 default 租户，零迁移（对齐 Go 的 deviceRecord.tenant()）。 */
function tenantOf(device: DeviceRecord): string {
  return device.tenant ?? DEFAULT_TENANT;
}

/** 一次性配对码：换一次设备令牌就作废；tenant 绑生成它的 approver 所在租户。 */
interface PairCodeRecord {
  expiresAt: number;
  tenant: string;
}

function pairCode(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return [...bytes].map((b) => PAIR_CODE_ALPHABET[b % PAIR_CODE_ALPHABET.length]).join("");
}

export interface Outcome {
  state: PendingState | "timeout";
  payload?: string;
  message?: string;
}

// /wait 挂在这里，/decide 把它唤醒：一次长轮询只花 2 个 subrequest（create + wait），
// 而不是每 2s 轮询一次（免费版 50 subrequests/请求 撑不住 300s TTL）。
const waiters = new Map<string, (outcome: Outcome) => void>();

function hex(bytes: Uint8Array): string {
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function randomId(): string {
  return hex(crypto.getRandomValues(new Uint8Array(16)));
}

function randomToken(): string {
  return hex(crypto.getRandomValues(new Uint8Array(24)));
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

function outcomeOf(record: PendingRecord): Outcome {
  return { state: record.state, payload: record.payload, message: record.message };
}

function pendingView(record: PendingRecord) {
  return {
    request_id: record.id,
    item: record.item,
    mode: record.mode,
    purpose: record.purpose,
    ttl: record.ttl,
    target: record.target || undefined,
    requester: record.requester || undefined,
    delivery: record.delivery || undefined,
    received_at: new Date(record.receivedAt).toISOString(),
    expires_at: new Date(record.expiresAt).toISOString(),
    state: "waiting",
    seal_public_key: record.sealPublicKey || undefined,
    public_key: record.publicKey || undefined,
    ssh_user: record.sshUser || undefined,
    cert_ttl: record.certTTL || undefined,
    device_id: record.deviceId || undefined,
    device_name: record.deviceName || undefined,
  };
}

export class BrokerState extends DurableObject<Env> {
  async fetch(request: Request): Promise<Response> {
    const path = new URL(request.url).pathname;
    const body = request.method === "POST" ? ((await request.json().catch(() => ({}))) as any) : {};
    switch (path) {
      case "/create":
        return json(200, await this.create(body));
      case "/wait":
        return json(200, await this.wait(body));
      case "/decide":
        return json(200, await this.decide(body));
      case "/fail":
        return json(200, await this.fail(body));
      case "/cancel":
        return json(200, await this.cancel(body));
      case "/detach":
        return json(200, await this.detach(body));
      case "/by-key":
        return json(200, await this.byKey(body));
      case "/list":
        return json(200, await this.list(body));
      case "/pair":
        return json(200, await this.pair(body));
      case "/pair-code":
        return json(200, await this.createPairCode(body));
      case "/claim":
        return json(200, await this.claimPairCode(body));
      case "/devices":
        return json(200, await this.listDevices(body));
      case "/revoke":
        return json(200, await this.revokeDevice(body));
      case "/rename":
        return json(200, await this.renameDevice(body));
      case "/renew":
        return json(200, await this.renewDevice(body));
      case "/push-token":
        return json(200, await this.setPushToken(body));
      case "/forget-device":
        return json(200, await this.forgetDevice(body));
      case "/device-authorized": {
        const device = await this.deviceByToken(String(body.deviceToken ?? ""));
        return json(
          200,
          device
            ? { ok: true, id: device.id, name: device.name, tenant: tenantOf(device) }
            : { ok: false },
        );
      }
      case "/approver-authorized": {
        // tenant 可选：给了就要求 approver 属于该租户（设备令牌路径的决定权按租户收）。
        const device = await this.deviceByToken(String(body.deviceToken ?? ""));
        const tenant = typeof body.tenant === "string" ? body.tenant : "";
        const ok =
          device !== null &&
          roleOf(device) === "approver" &&
          (tenant === "" || tenantOf(device) === tenant);
        return json(200, { ok, tenant: device ? tenantOf(device) : "" });
      }
      default:
        return json(404, { ok: false, reason: "unknown do route" });
    }
  }

  /** DO 的定时器：清掉过期记录，并唤醒还在等的长轮询。 */
  async alarm(): Promise<void> {
    const now = Date.now();
    let next: number | null = null;
    const records = await this.ctx.storage.list<PendingRecord>({ prefix: REQ_PREFIX });
    for (const [key, record] of records) {
      if (record.state !== "waiting") {
        await this.ctx.storage.delete(key);
        continue;
      }
      if (record.expiresAt <= now) {
        record.state = "expired";
        await this.ctx.storage.put(key, record);
        this.wake(record);
      } else if (record.detachedAt && now - record.detachedAt > DETACH_GRACE_MS) {
        // 断线宽限期过了还没重连：真取消，手机待批准随之消失
        await this.ctx.storage.delete(key);
      } else {
        // 宽限期到点也要跑一趟 alarm：detach 清理截止时间可能比 expiresAt 更早
        const deadline = record.detachedAt ? record.detachedAt + DETACH_GRACE_MS : record.expiresAt;
        if (next === null || Math.min(deadline, record.expiresAt) < next) {
          next = Math.min(deadline, record.expiresAt);
        }
      }
    }
    if (next !== null) await this.ctx.storage.setAlarm(next);
    // 顺手清掉过期的配对码（设备过期在 isDevice 里就地清理）
    const codes = await this.ctx.storage.list<PairCodeRecord>({ prefix: CODE_PREFIX });
    const stale = [...codes.entries()].filter(([, code]) => code.expiresAt <= now).map(([key]) => key);
    if (stale.length > 0) await this.ctx.storage.delete(stale);
  }

  /** 断线标记：连接断了不取消，记 detachedAt 进宽限期，等带同 key 的重连来清。 */
  private async detach(body: any): Promise<{ ok: boolean }> {
    const id = String(body.id ?? "");
    const record = await this.ctx.storage.get<PendingRecord>(REQ_PREFIX + id);
    if (!record || record.state !== "waiting") return { ok: false };
    record.detachedAt = Date.now();
    await this.ctx.storage.put(REQ_PREFIX + id, record);
    // 宽限期通常早于 expiresAt，得单独排个闹钟，不然清理要等到 TTL 才触发
    const current = await this.ctx.storage.getAlarm();
    const deadline = record.detachedAt + DETACH_GRACE_MS;
    if (current === null || deadline < current) {
      await this.ctx.storage.setAlarm(deadline);
    }
    return { ok: true };
  }

  /** 按幂等键找回活 pending：客户端重连用，同租户内匹配。 */
  private async byKey(body: any): Promise<{ id: string; record?: PendingRecord }> {
    const key = String(body.key ?? "");
    const tenant = String(body.tenant ?? DEFAULT_TENANT);
    if (key === "") return { id: "" };
    const records = await this.ctx.storage.list<PendingRecord>({ prefix: REQ_PREFIX });
    for (const record of records.values()) {
      if (
        record.requestKey === key &&
        (record.tenant || DEFAULT_TENANT) === tenant &&
        record.state === "waiting" &&
        record.expiresAt > Date.now()
      ) {
        record.detachedAt = 0;
        await this.ctx.storage.put(REQ_PREFIX + record.id, record);
        return { id: record.id, record };
      }
    }
    return { id: "" };
  }

  private async create(body: any): Promise<{ pushTokens: string[]; reused?: PendingRecord }> {
    const record = body.record as PendingRecord;
    // DO 单线程：查重+写入是原子的。同租户同 key 的活 pending 直接复用，不重开不重推。
    if (record.requestKey) {
      const existing = await this.byKey({ key: record.requestKey, tenant: record.tenant });
      if (existing.record) return { pushTokens: [], reused: existing.record };
    }
    await this.ctx.storage.put(REQ_PREFIX + record.id, record);
    const current = await this.ctx.storage.getAlarm();
    if (current === null || record.expiresAt < current) {
      await this.ctx.storage.setAlarm(record.expiresAt);
    }
    const devices = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
    const tenant = record.tenant || DEFAULT_TENANT;
    const pushTokens = [...devices.values()]
      .filter((device) => roleOf(device) === "approver" && tenantOf(device) === tenant)
      .map((device) => (device.pushToken ?? "").trim())
      .filter((token) => token !== "");
    return { pushTokens };
  }

  private async wait(body: any): Promise<Outcome> {
    const id = String(body.id ?? "");
    const timeoutMs = Math.max(1000, Number(body.timeoutMs ?? 60_000));
    const record = await this.ctx.storage.get<PendingRecord>(REQ_PREFIX + id);
    if (!record) return { state: "expired" };
    if (record.state !== "waiting") return outcomeOf(record);
    if (record.expiresAt <= Date.now()) return { state: "expired" };
    return await new Promise<Outcome>((resolve) => {
      let done = false;
      const finish = (outcome: Outcome) => {
        if (done) return;
        done = true;
        waiters.delete(id);
        resolve(outcome);
      };
      waiters.set(id, finish);
      setTimeout(() => finish({ state: "timeout" }), timeoutMs);
    });
  }

  private async decide(body: any): Promise<{ ok: boolean; reason?: string }> {
    const id = String(body.id ?? "");
    const decision = String(body.decision ?? "");
    const payload = typeof body.payload === "string" ? body.payload : "";
    const sig = typeof body.sig === "string" ? body.sig : "";
    const deviceToken = typeof body.deviceToken === "string" ? body.deviceToken : "";

    const record = await this.ctx.storage.get<PendingRecord>(REQ_PREFIX + id);
    if (!record || record.state !== "waiting" || record.expiresAt <= Date.now()) {
      return { ok: false, reason: "invalid or already used decision" };
    }
    const expected = decision === "deny" ? record.denySig : record.approveSig;
    const sigOK = sig !== "" && timingSafeEqual(sig, expected);
    // 设备令牌决定只能批自己租户的请求；FCM sig 绑定单个 request_id，别的租户拿不到。
    const deviceOK =
      deviceToken !== "" &&
      (await this.isApproverInTenant(deviceToken, record.tenant || DEFAULT_TENANT));
    if (!sigOK && !deviceOK) return { ok: false, reason: "invalid or already used decision" };
    // 手机密封的请求，设备批准时必须带回 v2 box（对齐 Go 的 claimDecision）
    if (decision === "approve" && record.sealPublicKey !== "" && payload === "" && !sigOK) {
      return { ok: false, reason: "invalid or already used decision" };
    }

    if (decision === "deny") {
      record.state = "denied";
    } else if (record.sealPublicKey !== "") {
      if (!payload.startsWith("v2.")) {
        record.state = "failed";
        record.message = "phone payload required";
      } else {
        record.state = "approved";
        record.payload = payload;
      }
    } else {
      // Go 版这里会从 rbw / SSH CA 取值；spike 不实现服务端取值路径
      record.state = "failed";
      record.message = "spike 不实现服务端取值，请让 CLI 带上 seal_public_key";
    }
    await this.ctx.storage.put(REQ_PREFIX + id, record);
    this.wake(record);
    return { ok: true };
  }

  private async fail(body: any): Promise<{ ok: boolean }> {
    const id = String(body.id ?? "");
    const record = await this.ctx.storage.get<PendingRecord>(REQ_PREFIX + id);
    if (!record || record.state !== "waiting") return { ok: false };
    record.state = "failed";
    record.message = String(body.message ?? "failed");
    await this.ctx.storage.put(REQ_PREFIX + id, record);
    this.wake(record);
    return { ok: true };
  }

  /** easyGet 断开后取消 pending，手机那边的待批准列表就跟着消失（对齐 Go 的 cancelPending）。 */
  private async cancel(body: any): Promise<{ ok: boolean }> {
    const id = String(body.id ?? "");
    const record = await this.ctx.storage.get<PendingRecord>(REQ_PREFIX + id);
    if (!record || record.state !== "waiting") return { ok: false };
    await this.ctx.storage.delete(REQ_PREFIX + id);
    return { ok: true };
  }

  private async list(body: any): Promise<unknown[]> {
    const now = Date.now();
    const tenant = typeof body.tenant === "string" ? body.tenant : "";
    const records = await this.ctx.storage.list<PendingRecord>({ prefix: REQ_PREFIX });
    return [...records.values()]
      .filter(
        (record) =>
          record.state === "waiting" &&
          record.expiresAt > now &&
          (tenant === "" || (record.tenant || DEFAULT_TENANT) === tenant),
      )
      .sort((a, b) => a.receivedAt - b.receivedAt)
      .map(pendingView);
  }

  private async pair(body: any): Promise<{ token: string; name: string }> {
    const raw = typeof body.name === "string" ? body.name.trim() : "";
    const name = raw === "" ? "phone" : raw.slice(0, 64);
    const vaultId = typeof body.vaultId === "string" ? body.vaultId.trim().slice(0, 64) : "";
    const token = randomToken();
    const device = this.newDevice(name, "approver");
    device.tenant = vaultId;
    // 独占语义：同租户同时只有一台 active approver——新手机配对上岗即撤销
    // 同租户其它 approver（换机踢下线）。requester 不受影响。
    const devices = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
    const evict = [...devices.entries()]
      .filter(
        ([, other]) =>
          roleOf(other) === "approver" && tenantOf(other) === tenantOf(device),
      )
      .map(([key]) => key);
    if (evict.length > 0) await this.ctx.storage.delete(evict);
    await this.ctx.storage.put(DEV_PREFIX + token, device);
    return { token, name };
  }

  private newDevice(name: string, role: "approver" | "requester"): DeviceRecord {
    const now = Date.now();
    return { id: randomId().slice(0, 8), name, role, created: now, expiresAt: now + DEVICE_TTL_MS };
  }

  /** 已授权设备生成一次性配对码；码本身不在设备表里，10 分钟过期；绑生成者的租户。 */
  private async createPairCode(body: any): Promise<{ code: string; expiresAt: number }> {
    const approver = await this.deviceByToken(String(body.deviceToken ?? ""));
    if (approver === null || roleOf(approver) !== "approver") return { code: "", expiresAt: 0 };
    const code = pairCode();
    const expiresAt = Date.now() + PAIR_CODE_TTL_MS;
    await this.ctx.storage.put(CODE_PREFIX + code, {
      expiresAt,
      tenant: tenantOf(approver),
    } satisfies PairCodeRecord);
    return { code, expiresAt };
  }

  /**
   * 用配对码换设备令牌。**先删码再用**，所以并发兑换只有一个成功；
   * 失败一律返回同一个 error，不区分「不存在 / 过期 / 已用过」，避免当探针。
   */
  private async claimPairCode(body: any): Promise<
    { token: string; name: string; expiresAt: number } | { error: string }
  > {
    const code = String(body.code ?? "").trim().toUpperCase();
    if (code === "") return { error: "invalid_claim" };
    const key = CODE_PREFIX + code;
    const record = await this.ctx.storage.get<PairCodeRecord>(key);
    if (!record) return { error: "invalid_claim" };
    await this.ctx.storage.delete(key);
    if (record.expiresAt <= Date.now()) return { error: "invalid_claim" };
    const raw = typeof body.name === "string" ? body.name.trim() : "";
    const name = raw === "" ? "device" : raw.slice(0, 64);
    const token = randomToken();
    const device = this.newDevice(name, "requester");
    // requester 继承配对码绑定的租户：claim 出的令牌固定归发码手机所在租户。
    device.tenant = record.tenant;
    await this.ctx.storage.put(DEV_PREFIX + token, device);
    return { token, name, expiresAt: device.expiresAt };
  }

  /** 设备列表：不返回令牌本体，只给 id 与元数据；只见自己租户。 */
  private async listDevices(body: any): Promise<{ unauthorized?: boolean; devices?: unknown[] }> {
    const current = String(body.deviceToken ?? "");
    const caller = await this.deviceByToken(current);
    if (caller === null || roleOf(caller) !== "approver") return { unauthorized: true };
    const now = Date.now();
    const list = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
    const devices = [...list.entries()]
      .filter(([, device]) => device.expiresAt > now && tenantOf(device) === tenantOf(caller))
      .map(([key, device]) => ({
        id: device.id,
        name: device.name,
        role: roleOf(device),
        created_at: device.created,
        last_used_at: device.lastUsedAt ?? null,
        expires_at: device.expiresAt,
        current: key === DEV_PREFIX + current,
      }))
      .sort((a, b) => a.created_at - b.created_at);
    return { devices };
  }

  /**
   * 撤销设备。**允许撤销自己**——这就是「登出这台设备」；否则设备表里的最后一台
   * 永远删不掉，只能走 admin 后门。App 侧撤销自己后要清掉本地令牌。
   */
  private async revokeDevice(body: any): Promise<{ unauthorized?: boolean; removed?: number }> {
    const current = String(body.deviceToken ?? "");
    const caller = await this.deviceByToken(current);
    if (caller === null || roleOf(caller) !== "approver") return { unauthorized: true };
    const id = String(body.id ?? "");
    if (id === "") return { removed: 0 };
    const list = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
    for (const [key, device] of list) {
      // 只能撤自己租户的设备；别的租户的 id 存在也当不存在。
      if (device.id === id && tenantOf(device) === tenantOf(caller)) {
        await this.ctx.storage.delete(key);
        return { removed: 1 };
      }
    }
    return { removed: 0 };
  }

  /** 改名设备。同租户规则同撤销：别的租户的 id 存在也当不存在。 */
  private async renameDevice(body: any): Promise<{ unauthorized?: boolean; renamed?: number }> {
    const caller = await this.deviceByToken(String(body.deviceToken ?? ""));
    if (caller === null || roleOf(caller) !== "approver") return { unauthorized: true };
    const id = String(body.id ?? "");
    const name = String(body.name ?? "").trim();
    if (id === "" || name === "") return { renamed: 0 };
    const list = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
    for (const [key, device] of list) {
      if (device.id === id && tenantOf(device) === tenantOf(caller)) {
        device.name = name;
        await this.ctx.storage.put(key, device);
        return { renamed: 1 };
      }
    }
    return { renamed: 0 };
  }

  /** 续期：把自己这台再延 180 天。 */
  private async renewDevice(body: any): Promise<{ unauthorized?: boolean; expiresAt?: number }> {
    const token = String(body.deviceToken ?? "");
    if (!(await this.isApprover(token))) return { unauthorized: true };
    const key = DEV_PREFIX + token;
    const record = await this.ctx.storage.get<DeviceRecord>(key);
    if (!record) return { unauthorized: true };
    record.expiresAt = Date.now() + DEVICE_TTL_MS;
    await this.ctx.storage.put(key, record);
    return { expiresAt: record.expiresAt };
  }

  private async setPushToken(body: any): Promise<{ ok: boolean }> {
    const token = String(body.deviceToken ?? "");
    const pushToken = String(body.token ?? "").trim();
    if (pushToken === "" || !(await this.isApprover(token))) return { ok: false };
    const record = await this.ctx.storage.get<DeviceRecord>(DEV_PREFIX + token);
    if (!record) return { ok: false };
    record.pushToken = pushToken;
    await this.ctx.storage.put(DEV_PREFIX + token, record);
    return { ok: true };
  }

  /**
   * 解绑设备（手机丢了、或清掉测试设备）。
   * body.deviceToken 指定一台；body.all = true 清空整个设备表。
   */
  private async forgetDevice(body: any): Promise<{ removed: number }> {
    if (body.all === true) {
      const devices = await this.ctx.storage.list<DeviceRecord>({ prefix: DEV_PREFIX });
      const keys = [...devices.keys()];
      if (keys.length > 0) await this.ctx.storage.delete(keys);
      return { removed: keys.length };
    }
    const token = String(body.deviceToken ?? "");
    if (token === "" || !(await this.isDevice(token))) return { removed: 0 };
    await this.ctx.storage.delete(DEV_PREFIX + token);
    return { removed: 1 };
  }

  /** 设备记录：存在且没过期才算有效。顺手节流记 lastUsedAt。 */
  private async deviceByToken(token: string): Promise<DeviceRecord | null> {
    if (token === "" || /[\s]/.test(token)) return null;
    const key = DEV_PREFIX + token;
    const record = await this.ctx.storage.get<DeviceRecord>(key);
    if (!record) return null;
    const now = Date.now();
    if (record.expiresAt <= now) {
      await this.ctx.storage.delete(key);
      return null;
    }
    if (now - (record.lastUsedAt ?? record.created) > LAST_USED_TOUCH_MS) {
      record.lastUsedAt = now;
      await this.ctx.storage.put(key, record);
    }
    return record;
  }

  /** 设备是否仍然有效（不看角色）：/v1/request 用它。 */
  private async isDevice(token: string): Promise<boolean> {
    return (await this.deviceByToken(token)) !== null;
  }

  /** 只有 approver 能批准、管设备；requester 只能发起请求（对齐 Go 的 approverAuthorizedLocked）。 */
  private async isApprover(token: string): Promise<boolean> {
    const record = await this.deviceByToken(token);
    return record !== null && roleOf(record) === "approver";
  }

  /** approver 且属于指定租户：设备令牌路径的决定权按租户收（对齐 Go 的 approverAuthorizedTenantLocked）。 */
  private async isApproverInTenant(token: string, tenant: string): Promise<boolean> {
    const record = await this.deviceByToken(token);
    return record !== null && roleOf(record) === "approver" && tenantOf(record) === tenant;
  }

  private wake(record: PendingRecord): void {
    const waiter = waiters.get(record.id);
    if (waiter) waiter(outcomeOf(record));
  }
}
