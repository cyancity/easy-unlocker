"use strict";

// app.js — 状态、路由、各页逻辑与启动。
let state = { status: null, items: [], pending: [] };
let page = "approve";
let qrPoll = null;
let brokerOnline = true;
let reqTtl = 900;
// ---------- 路由与门禁 ----------

function goto(p) {
  page = p;
  document.body.classList.remove("navless");
  document.querySelectorAll(".page").forEach((el) => el.classList.remove("active"));
  document.querySelectorAll(".nav-btn").forEach((el) => el.classList.toggle("active", el.dataset.page === p));
  $("page-" + p).classList.add("active");
  $("main").scrollTop = 0;
  if (p === "approve") renderPending();
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
  $("lock-sub").textContent = s && s.vault_state === "none"
    ? "本机还没有库。先从服务器同步一次，再用手机上设的密码解锁。"
    : "解锁一次，直到锁屏、重启或手动锁上。";
  $("lock-msg").textContent = "";
  setTimeout(() => $("lock-input").focus(), 60);
  // 缓存的密文可能落后（手机上改密码/加条目）——进门禁页静默拉最新。
  if (s && s.paired) A.SyncVault().then(refreshStatus).catch(() => {});
}

function showSetup() {
  page = "setup";
  document.body.classList.add("navless");
  document.querySelectorAll(".page").forEach((el) => el.classList.remove("active"));
  $("page-setup").classList.add("active");
  if (state.status && state.status.broker_url) $("pair-broker").value = state.status.broker_url;
}

// ---------- 状态与侧栏 ----------

const ROLE_NAMES = { approver: "批准端", requester: "取凭据端" };

async function refreshStatus() {
  state.status = await A.Status();
  const s = state.status;
  const el = $("status");
  if (!s.paired) {
    el.innerHTML = `<span class="dot off"></span><div class="status-main"><b>未配对</b></div>`;
  } else {
    const vaultChip = s.vault_state === "unlocked"
      ? `<span class="chip ok">库已解锁</span>`
      : s.vault_state === "locked" ? `<span class="chip warn-chip">库已锁</span>` : `<span class="chip">无本地库</span>`;
    el.innerHTML = `<span class="dot ${brokerOnline ? "on" : "off"}"></span>
      <div class="status-main">
        <b>${esc(s.device_name || "本机")}</b>
        <span class="status-roles"><span class="chip accent">${esc(ROLE_NAMES[s.role] || s.role || "?")}</span>${vaultChip}</span>
        <div class="status-ver" title="${esc(s.broker_url)}">${esc(s.version || "dev")}${brokerOnline ? "" : " · 网关不可达"}</div>
      </div>`;
  }
  const lockBadge = $("lock-badge");
  lockBadge.hidden = !(s.paired && s.vault_state === "locked");
  return s;
}

// ---------- 配对 ----------

// 单字段表单无 submit 按钮：不拦住 Enter 会触发隐式提交整页重载。
// Enter = 主操作（生成二维码）。
$("pair-form").onsubmit = (e) => { e.preventDefault(); $("qr-btn").click(); };

$("qr-btn").onclick = async () => {
  const broker = $("pair-broker").value.trim();
  if (!broker) return toast("先填 Broker 地址", true);
  try {
    const uri = await call("StartQRPair", broker);
    $("qr-img").src = uri;
    $("qr-area").hidden = false;
    $("manual-area").hidden = true;
    $("qr-msg").textContent = "等待手机扫码";
    clearInterval(qrPoll);
    qrPoll = setInterval(async () => {
      try {
        if ((await A.PollQRPair()) === "paired") {
          clearInterval(qrPoll);
          await refreshStatus();
          toast("配对成功");
          if (gated()) showLock(); else goto("approve");
        }
      } catch (e) { clearInterval(qrPoll); $("qr-msg").textContent = "配对中断，重新生成二维码"; }
    }, 2000);
  } catch (e) {}
};

$("show-manual").onclick = () => {
  clearInterval(qrPoll);
  A.CancelQRPair().catch(() => {});
  $("qr-area").hidden = true;
  $("manual-area").hidden = false;
  $("pair-code").focus();
};

$("manual-area").onsubmit = async (e) => {
  e.preventDefault();
  const broker = $("pair-broker").value.trim();
  const code = $("pair-code").value.trim();
  if (!broker || !code) return toast("地址和配对码都要填", true);
  $("pair-btn").disabled = true;
  $("pair-msg").textContent = "配对中";
  try {
    const s = await call("Pair", broker, code);
    await refreshStatus();
    toast(`配对成功，这台电脑是${ROLE_NAMES[s.role] || s.role}`);
    if (gated()) showLock(); else goto("approve");
  } catch (e2) {
    $("pair-msg").textContent = "";
  } finally {
    $("pair-btn").disabled = false;
  }
};

// ---------- 解锁 ----------

async function doUnlock() {
  const input = $("lock-input"), msg = $("lock-msg");
  const v = input.value;
  if (!v) { msg.textContent = "输入解锁密码或恢复码"; return; }
  msg.textContent = "";
  $("lock-unlock-btn").disabled = true;
  try {
    let n;
    try { n = await A.Unlock(v); }
    catch (e1) {
      // 本地密文可能落后于手机（改了密码/条目）——拉一次再试一遍。
      let retryOk = false;
      try { if (await A.SyncVault()) { n = await A.Unlock(v); retryOk = true; } } catch (e2) {}
      if (!retryOk) throw e1;
    }
    input.value = "";
    toast(`解锁成功，${n} 条`);
    await refreshStatus();
    goto("approve");
  } catch (e) {
    msg.innerHTML = `<span class="danger-text">${esc(String(e.message || e).replace(/^Error:\s*/, ""))}</span>`;
  } finally {
    $("lock-unlock-btn").disabled = false;
  }
}
$("lock-form").onsubmit = (e) => { e.preventDefault(); doUnlock(); };
$("lock-sync-btn").onclick = doSync;
$("goto-lock-btn").onclick = () => showLock();

// ---------- 库 ----------

async function renderVault() {
  const s = await refreshStatus();
  const locked = s.vault_state !== "unlocked";
  $("vault-locked").hidden = locked ? false : true;
  $("vault-open").hidden = locked;
  if (locked) return;
  const items = await A.Items().catch(() => []);
  state.items = items;
  const meta = [`${items.length} 条`];
  if (s.synced_at) meta.push(`同步于 ${esc(new Date(s.synced_at).toLocaleString())}`);
  $("vault-meta").innerHTML = meta.join('<span class="sep"></span>');
  renderVaultRows($("vault-search").value.trim());
}

function renderVaultRows(q) {
  const items = state.items.filter((i) =>
    !q || i.name.toLowerCase().includes(q.toLowerCase()) || (i.aliases || []).some((a) => a.toLowerCase().includes(q.toLowerCase())));
  const wrap = $("item-list");
  if (!items.length) {
    wrap.innerHTML = `<div class="empty"><div class="glyph">${ic("vault")}</div>
      ${state.items.length ? "没有匹配的条目" : "库里还没有条目"}<div class="hint">${state.items.length ? "" : "在手机 App 里添加，然后点右上角同步"}</div></div>`;
    return;
  }
  wrap.innerHTML = `<div class="card" style="padding:6px 8px"><table><thead>
    <tr><th>名称</th><th>别名</th><th class="ta-r">复制</th></tr></thead><tbody>` +
    items.map((i) => `<tr>
      <td><span class="name">${esc(i.name)}</span></td>
      <td><span class="aliases">${esc((i.aliases || []).join("、"))}</span></td>
      <td><div class="cellbtns">
        ${i.has_value ? `<button type="button" class="iconbtn" data-n="${esc(i.name)}" data-f="secret" title="复制密码" aria-label="复制密码">${ic("copy")}</button>` : ""}
        ${i.has_note ? `<button type="button" class="iconbtn" data-n="${esc(i.name)}" data-f="note" title="复制备注" aria-label="复制备注">${ic("pencil")}</button>` : ""}
      </div></td>
    </tr>`).join("") + `</tbody></table></div>`;
  wrap.querySelectorAll(".iconbtn").forEach((b) => {
    b.onclick = async () => {
      try {
        await call("CopyItem", b.dataset.n, b.dataset.f);
        b.classList.add("done");
        b.innerHTML = ic("check");
        toast(`已复制${b.dataset.f === "note" ? "备注" : "密码"}`);
        setTimeout(() => { b.classList.remove("done"); b.innerHTML = ic(b.dataset.f === "note" ? "pencil" : "copy"); }, 1400);
      } catch (e) {}
    };
  });
}
$("vault-search").oninput = (e) => renderVaultRows(e.target.value.trim());

async function doSync() {
  try {
    const got = await call("SyncVault");
    toast(got ? "已从服务器同步" : "服务器上还没有库");
    await refreshStatus();
    if (gated()) showLock(); else if (page === "vault") renderVault();
  } catch (e) {}
}
$("sync-btn").onclick = doSync;
$("sync-btn-2").onclick = doSync;
$("lock-btn").onclick = async () => {
  await call("Lock");
  toast("库已锁上");
  await refreshStatus();
  showLock();
};

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

$("req-ttl-seg").querySelectorAll(".seg-btn").forEach((b) => {
  b.onclick = () => {
    $("req-ttl-seg").querySelectorAll(".seg-btn").forEach((x) => x.classList.remove("active"));
    b.classList.add("active");
    const custom = b.dataset.ttl === "custom";
    $("req-ttl-custom").hidden = !custom;
    if (custom) $("req-ttl").focus();
    else reqTtl = +b.dataset.ttl;
  };
});

$("req-form").onsubmit = async (e) => {
  e.preventDefault();
  const item = $("req-item").value.trim();
  if (!item) return toast("填条目名", true);
  const custom = $("req-ttl-seg").querySelector(".active").dataset.ttl === "custom";
  const ttl = custom ? (+$("req-ttl").value || 300) : reqTtl;
  $("req-btn").disabled = true;
  $("req-msg").textContent = "等待批准，可在手机或其它设备上批";
  try {
    const msg = await call("RequestValue", item, $("req-purpose").value.trim(), ttl);
    $("req-msg").textContent = "";
    toast(msg || "已复制到剪贴板");
    $("req-purpose").value = "";
  } catch (e2) {
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
  try { devices = await call("Devices"); } catch (e) { return; }
  const tbody = $("device-table").querySelector("tbody");
  tbody.innerHTML = devices.map((d) => `<tr>
    <td>${esc(d.name)}${d.current ? ' <span class="small ok">本机</span>' : ""}</td>
    <td><span class="chip ${d.role === "approver" ? "accent" : ""}">${esc(ROLE_NAMES[d.role] || d.role || "?")}</span></td>
    <td class="small muted">${relTime(d.last_used_at)}</td>
    <td><div class="cellbtns">
      ${state.status?.role === "approver" && !d.current ? `<button type="button" class="iconbtn" data-rename="${esc(d.id)}" data-name="${esc(d.name)}" title="重命名" aria-label="重命名">${ic("pencil")}</button>` : ""}
      ${!d.current ? `<button type="button" class="iconbtn danger" data-revoke="${esc(d.id)}" data-name="${esc(d.name)}" title="撤销" aria-label="撤销">${ic("x")}</button>` : ""}
    </div></td>
  </tr>`).join("");
  tbody.querySelectorAll("[data-revoke]").forEach((b) => {
    b.onclick = async () => {
      if (!await dialog({ title: "撤销设备", body: `撤销后 <b>${esc(b.dataset.name)}</b> 立即失能，需要重新配对。`, confirmText: "撤销", danger: true })) return;
      try { await call("RevokeDevice", b.dataset.revoke); toast("已撤销"); refreshDevices(); } catch (e) {}
    };
  });
  tbody.querySelectorAll("[data-rename]").forEach((b) => {
    b.onclick = async () => {
      const name = await dialogPrompt({ title: "重命名设备", label: "设备名称", value: b.dataset.name });
      if (!name) return;
      try { await call("RenameDevice", b.dataset.rename, name); toast("已改名"); refreshDevices(); } catch (e) {}
    };
  });
}

$("unpair-btn").onclick = async () => {
  if (!await dialog({ title: "解除本机配对", body: "本机令牌作废、本地库缓存清空，等于退出登录。", confirmText: "解除配对", danger: true })) return;
  try { await call("Unpair"); location.reload(); } catch (e) {}
};

// ---------- 导航与启动 ----------

document.querySelectorAll(".nav-btn").forEach((b) => {
  b.onclick = () => {
    if (!state.status || !state.status.paired) return toast("先配对再使用", true);
    if (gated()) { toast("库已锁上，先解锁", true); showLock(); return; }
    goto(b.dataset.page);
  };
});

// 批准页 3s 跟手；其它页 9s 足够 badge 用——后端 watchPending 本就 3s 打同一端点。
let pollTick = 0;
setInterval(() => {
  if (!state.status?.paired) return;
  pollTick++;
  if (page !== "approve" && pollTick % 3 !== 0) return;
  refreshPending();
}, 3000);

if (window.runtime && window.runtime.EventsOn) {
  // 后台在系统锁屏时锁库并发事件——前端刷状态并回到门禁页。
  window.runtime.EventsOn("vault:locked", () => {
    toast("屏幕已锁定，库已重新上锁");
    refreshStatus().then(() => { if (gated()) showLock(); });
  });
  // 新请求到达（后台 3s 扫描发现）：跳到批准页并立即拉列表。
  window.runtime.EventsOn("pending:new", () => {
    if (!state.status?.paired) return;
    if (gated()) return;
    if (page !== "approve") goto("approve");
    refreshPending();
  });
}

(async function boot() {
  try {
    await refreshStatus();
    const s = state.status;
    if (!s.paired) {
      showSetup();
    } else if (gated()) {
      showLock();
      A.SyncVault().catch(() => {});
    } else {
      goto("approve");
      await refreshPending();
      A.SyncVault().catch(() => {});
    }
  } catch (e) {
    showSetup();
  }
})();
