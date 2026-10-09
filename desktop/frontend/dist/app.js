"use strict";

// Wails 绑定：window.go.main.App.<Method>(...) → Promise
const A = window.go && window.go.main && window.go.main.App;

let state = { paired: false, role: "", vault: "none", items: [] };
let page = "approve";
let pollTimer = null;

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

function toast(msg, isErr) {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast show" + (isErr ? " err" : "");
  setTimeout(() => (t.className = "toast"), 3200);
}

function call(fn, ...args) {
  return A[fn](...args).catch((e) => {
    const msg = (e && e.toString()) || "调用失败";
    toast(msg.replace(/^Error: /, ""), true);
    throw e;
  });
}

// ---------- 路由 ----------

function goto(p) {
  page = p;
  document.body.classList.remove("navless");
  document.querySelectorAll(".page").forEach((el) => el.classList.remove("active"));
  document.querySelectorAll(".nav-btn").forEach((el) => el.classList.toggle("active", el.dataset.page === p));
  $("page-" + p).classList.add("active");
  if (p === "approve") refreshPending();
  if (p === "vault") renderVault();
  if (p === "devices") refreshDevices();
  if (p === "request") refreshItemHints();
}

// approver 配对后库未解锁 → 整个应用只剩解锁页（session lock 的入口门禁）。
function gated() {
  const s = state.status;
  return !!(s && s.paired && s.role === "approver" && s.vault_state !== "unlocked");
}

function showLock() {
  page = "lock";
  document.body.classList.add("navless");
  document.querySelectorAll(".page").forEach((el) => el.classList.remove("active"));
  document.querySelectorAll(".nav-btn").forEach((el) => el.classList.remove("active"));
  $("page-lock").classList.add("active");
  const s = state.status;
  $("lock-sub").textContent = s.vault_state === "none"
    ? "本地还没有库——先从服务器同步一次，再用密码解锁。"
    : "本 session 只需解锁一次——锁屏、重启或退出后才会重新上锁。";
  $("lock-msg").textContent = "";
  setTimeout(() => $("lock-input").focus(), 50);
  // 缓存的 blob 可能落后（手机上改密码/加条目）——进门禁页静默拉最新。
  if (s.paired) {
    A.SyncVault().then(async () => { await refreshStatus(); }).catch(() => {});
  }
}

function showSetup() {
  page = "setup";
  document.body.classList.add("navless");
  document.querySelectorAll(".page").forEach((el) => el.classList.remove("active"));
  document.querySelectorAll(".nav-btn").forEach((el) => el.classList.remove("active"));
  $("page-setup").classList.add("active");
}

async function refreshStatus() {
  state.status = await A.Status();
  const s = state.status;
  const dot = $("status-dot");
  dot.innerHTML = s.paired
    ? `<span class="dot on"></span><b>${esc(s.device_name || "本机")}</b><br/>${esc(s.role || "?")} · ${esc(s.vault_state)}`
    : `<span class="dot off"></span>未配对`;
  return s;
}

// ---------- 配对 ----------

// ---------- 扫码配对 ----------

let qrPoll = null;

$("qr-btn").onclick = async () => {
  const broker = $("pair-broker").value.trim();
  if (!broker) return toast("先填 broker 地址", true);
  try {
    const uri = await A.StartQRPair(broker);
    $("qr-img").src = uri;
    $("qr-area").style.display = "";
    $("manual-area").style.display = "none";
    $("qr-msg").textContent = "等待手机扫码…";
    clearInterval(qrPoll);
    qrPoll = setInterval(async () => {
      try {
        const st = await A.PollQRPair();
        if (st === "paired") {
          clearInterval(qrPoll);
          await refreshStatus();
          toast("配对成功");
          if (gated()) showLock(); else goto("approve");
        }
      } catch (e) {
        clearInterval(qrPoll);
        $("qr-msg").textContent = "";
      }
    }, 2000);
  } catch (e) {}
};

$("show-manual").onclick = () => {
  clearInterval(qrPoll);
  A.CancelQRPair().catch(() => {});
  $("qr-area").style.display = "none";
  $("manual-area").style.display = "";
};

$("pair-btn").onclick = async () => {
  const broker = $("pair-broker").value.trim();
  const code = $("pair-code").value.trim();
  if (!broker || !code) return toast("地址和配对码都要填", true);
  $("pair-btn").disabled = true;
  $("pair-msg").textContent = "配对中…";
  try {
    const s = await A.Pair(broker, code);
    await refreshStatus();
    toast("配对成功：" + (s.role === "approver" ? "批准端" : "取凭据端"));
    if (gated()) showLock(); else goto("approve");
  } catch (e) {
    $("pair-msg").textContent = "";
  } finally {
    $("pair-btn").disabled = false;
  }
};

// ---------- 待批准 ----------

async function refreshPending() {
  if (!state.status || !state.status.paired) return;
  let list = [];
  try { list = await A.Pending(); } catch (e) { return; }
  state.pending = list;
  const badge = $("pending-badge");
  badge.style.display = list.length ? "" : "none";
  badge.textContent = list.length;
  const wrap = $("pending-list");
  if (!list.length) {
    wrap.innerHTML = `<div class="empty"><div class="big">○</div>没有等待批准的请求</div>`;
    return;
  }
  wrap.innerHTML = list.map((r) => `
    <div class="card req-card">
      <div class="row">
        <span class="item">${esc(r.item)}</span>
        <span class="ttl">剩 ${Math.max(0, Math.round((new Date(r.expires_at) - Date.now()) / 1000))}s</span>
      </div>
      <div class="meta">
        来源 <b>${esc(r.requester || "?")}</b> · 用途 <b>${esc(r.purpose || "未说明")}</b>
        ${r.mode === "sign" ? ' · <span class="warn">签名模式</span>' : ""}
      </div>
      <div class="actions">
        ${r.item === "#items"
          ? `<span class="small muted">请求的是条目清单</span>`
          : `<select class="approve-item" data-rid="${esc(r.request_id)}"></select>
             <select class="approve-field">
               <option value="secret">密码</option>
               <option value="note">备注</option>
             </select>`}
        <button class="btn approve-btn" data-rid="${esc(r.request_id)}">批准</button>
        <button class="btn danger deny-btn" data-rid="${esc(r.request_id)}">拒绝</button>
      </div>
    </div>`).join("");
  // 填充条目下拉：默认精确命中，其余按名字排序。
  let names = [];
  if (state.status.vault_state === "unlocked") {
    try { names = (await A.Items()).map((i) => i.name); } catch (e) {}
  }
  wrap.querySelectorAll("select.approve-item").forEach((sel) => {
    const rid = sel.dataset.rid;
    const want = list.find((r) => r.request_id === rid)?.item;
    const opts = [want, ...names.filter((n) => n !== want)];
    sel.innerHTML = opts.map((n) => `<option>${esc(n)}</option>`).join("");
  });
  wrap.querySelectorAll(".approve-btn").forEach((b) => {
    b.onclick = async () => {
      b.disabled = true;
      const card = b.closest(".req-card");
      const item = card.querySelector(".approve-item")?.value || "";
      const field = card.querySelector(".approve-field")?.value || "secret";
      try {
        await A.Approve(b.dataset.rid, item, field);
        toast("已批准");
        refreshPending();
      } catch (e) { toast(String(e), true); b.disabled = false; }
    };
  });
  wrap.querySelectorAll(".deny-btn").forEach((b) => {
    b.onclick = async () => {
      b.disabled = true;
      try { await A.Deny(b.dataset.rid); toast("已拒绝"); refreshPending(); }
      catch (e) { toast(String(e), true); b.disabled = false; }
    };
  });
}

// ---------- 锁屏门禁 ----------

async function doUnlock(inputEl, msgEl) {
  const v = inputEl.value;
  if (!v) { msgEl.textContent = "输入解锁密码或恢复码"; return; }
  msgEl.textContent = "";
  try {
    const n = await A.Unlock(v);
    inputEl.value = "";
    toast(`解锁成功，${n} 条`);
    await refreshStatus();
    goto("approve");
  } catch (e) {
    // 本地 blob 可能落后于手机（改了密码/条目）——拉一次再试一遍。
    try { if (await A.SyncVault()) { try { const n = await A.Unlock(v); inputEl.value = ""; toast(`解锁成功，${n} 条`); await refreshStatus(); goto("approve"); return; } catch (e2) { e = e2; } } } catch (e2) {}
    msgEl.innerHTML = `<span class="danger-text">${esc(String(e).replace(/^Error: /, ""))}</span>`;
  }
}

$("lock-unlock-btn").onclick = () => doUnlock($("lock-input"), $("lock-msg"));
$("lock-input").addEventListener("keydown", (e) => { if (e.key === "Enter") $("lock-unlock-btn").click(); });
$("lock-sync-btn").onclick = doSync;
$("goto-lock-btn").onclick = () => showLock();

// ---------- 库 ----------

async function renderVault() {
  const s = await refreshStatus();
  const locked = s.vault_state !== "unlocked";
  $("vault-locked").style.display = locked ? "" : "none";
  $("vault-open").style.display = locked ? "none" : "";
  $("vault-sub").textContent = locked ? "" : "";
  if (locked) return;
  $("vault-meta").innerHTML = `vault <b class="mono">${esc(s.vault_id || "?")}</b> · ${s.vault_items} 条` +
    (s.synced_at ? ` · 同步于 ${esc(new Date(s.synced_at).toLocaleString())}` : "");
  const items = await A.Items().catch(() => []);
  state.items = items;
  $("item-list").innerHTML = items.length
    ? `<table><thead><tr><th>名称</th><th>别名</th><th>内容</th><th></th></tr></thead><tbody>` +
      items.map((i) => `<tr>
        <td class="mono">${esc(i.name)}</td>
        <td class="muted">${esc((i.aliases || []).join(", "))}</td>
        <td class="small muted">${i.has_value ? "密码" : ""}${i.has_value && i.has_note ? " · " : ""}${i.has_note ? "备注" : ""}</td>
        <td>
          ${i.has_value ? `<button class="link copy-btn" data-n="${esc(i.name)}" data-f="secret">复制密码</button>` : ""}
          ${i.has_note ? `<button class="link copy-btn" data-n="${esc(i.name)}" data-f="note">复制备注</button>` : ""}
        </td>
      </tr>`).join("") + `</tbody></table>`
    : `<div class="empty"><div class="big">○</div>库里是空的</div>`;
  document.querySelectorAll(".copy-btn").forEach((b) => {
    b.onclick = async () => {
      try { await A.CopyItem(b.dataset.n, b.dataset.f); toast("已复制"); } catch (e) {}
    };
  });
}

async function doSync() {
  try {
    const got = await A.SyncVault();
    toast(got ? "已从服务器同步" : "服务器上还没有库");
    await refreshStatus();
    if (gated()) showLock(); else if (page === "vault") renderVault();
  } catch (e) {}
}
$("sync-btn").onclick = doSync;
$("sync-btn-2").onclick = doSync;
$("lock-btn").onclick = async () => { await A.Lock(); toast("已锁上"); await refreshStatus(); showLock(); };

// ---------- 取密 ----------

async function refreshItemHints() {
  const dl = $("req-item-list");
  if (state.status?.vault_state === "unlocked") {
    const items = state.items.length ? state.items : await A.Items().catch(() => []);
    dl.innerHTML = items.map((i) => `<option value="${esc(i.name)}">`).join("");
  } else {
    dl.innerHTML = `<option value="#items">`;
  }
}

$("req-btn").onclick = async () => {
  const item = $("req-item").value.trim();
  if (!item) return toast("填条目名", true);
  $("req-btn").disabled = true;
  $("req-msg").textContent = "等待批准…（可在手机/其它桌面批准）";
  try {
    const msg = await A.RequestValue(item, $("req-purpose").value.trim(), +$("req-ttl").value || 300);
    $("req-msg").textContent = "";
    toast(msg);
  } catch (e) {
    $("req-msg").textContent = "";
  } finally {
    $("req-btn").disabled = false;
  }
};

// ---------- 设备 ----------

async function refreshDevices() {
  const s = await refreshStatus();
  $("self-name").textContent = s.device_name || "本机";
  let devices = [];
  try { devices = await A.Devices(); } catch (e) { return; }
  const tbody = $("device-table").querySelector("tbody");
  tbody.innerHTML = devices.map((d) => `<tr>
    <td>${esc(d.name)}${d.current ? ' <span class="small ok">← 本机</span>' : ""}</td>
    <td><span class="role ${esc(d.role)}">${esc(d.role || "?")}</span></td>
    <td class="small muted">${d.last_seen ? esc(new Date(d.last_seen).toLocaleString()) : "—"}</td>
    <td>
      ${!d.current ? `<button class="link danger revoke-btn" data-id="${esc(d.id)}">撤销</button>` : ""}
    </td>
  </tr>`).join("");
  tbody.querySelectorAll(".revoke-btn").forEach((b) => {
    b.onclick = async () => {
      if (!confirm("撤销后该设备立即失能，确定？")) return;
      try { await A.RevokeDevice(b.dataset.id); toast("已撤销"); refreshDevices(); } catch (e) {}
    };
  });
}

$("unpair-btn").onclick = async () => {
  if (!confirm("解除本机配对：令牌作废、本地库清空。确定？")) return;
  try { await A.Unpair(); location.reload(); } catch (e) {}
};

// ---------- 导航与启动 ----------

document.querySelectorAll(".nav-btn").forEach((b) => {
  b.onclick = () => {
    if (!state.status || !state.status.paired) {
      toast("先配对再使用", true);
      return;
    }
    if (gated()) {
      toast("库已锁上——先解锁", true);
      showLock();
      return;
    }
    goto(b.dataset.page);
  };
});

setInterval(() => { if (page === "approve" && state.status?.paired) refreshPending(); }, 3000);

// 后端在系统锁屏时把库锁上并发来事件——前端刷状态并提示。
if (window.runtime && window.runtime.EventsOn) {
  window.runtime.EventsOn("vault:locked", () => {
    toast("屏幕已锁定，库已重新上锁");
    refreshStatus().then(() => { if (gated()) showLock(); });
  });
}

(async function boot() {
  try {
    const s = await refreshStatus();
    if (!s.paired) {
      showSetup();
      if (s.broker_url) $("pair-broker").value = s.broker_url;
    } else if (gated()) {
      showLock();
      A.SyncVault().catch(() => {});
    } else {
      goto("approve");
      A.SyncVault().catch(() => {});
    }
  } catch (e) {
    showSetup();
  }
})();
