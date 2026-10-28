"use strict";

// approve.js — 待批准页渲染：卡片、倒计时、智能默认（有密码放密码，纯备注放备注）。

// ---------- 待批准 ----------

let lastPendingKey = "";

function pendingCard(r, freshIds) {
  const ttl = r.ttl > 0 ? r.ttl : 300;
  const meta = `
    <div><dt>来源</dt><dd class="mono">${esc(r.requester || "?")}</dd></div>
    <div><dt>用途</dt><dd>${esc(r.purpose || "未说明")}</dd></div>`;
  const chips = r.mode === "sign" ? `<span class="chip warn-chip">SSH 签名</span>` : "";
  const isItemsReq = r.item === "#items";
  const body = isItemsReq
    ? `<div class="req-actions">
         <span class="small muted">请求条目清单，只有名称不含值</span>
         <button type="button" class="btn primary approve-btn">${ic("check")}批准并放出</button>
         <button type="button" class="btn quiet danger deny-btn">拒绝</button>
       </div>`
    : `<div class="req-actions">
         <button type="button" class="btn primary approve-btn">${ic("check")}批准并放出</button>
         <button type="button" class="btn quiet danger deny-btn">拒绝</button>
         <button type="button" class="linkbtn alt-toggle">换一条条目</button>
       </div>
       <div class="req-alt" hidden>
         <select class="approve-item" aria-label="选哪条条目"></select>
         <div class="seg small" role="group" aria-label="放出哪栏">
           <button type="button" class="seg-btn active" data-field="secret">密码</button>
           <button type="button" class="seg-btn" data-field="note">备注</button>
         </div>
       </div>`;
  return `<article class="req-card${freshIds.includes(r.request_id) ? " fresh" : ""}" data-rid="${esc(r.request_id)}" data-exp="${esc(r.expires_at)}" data-ttl="${ttl}">
    <div class="req-top">
      <span class="req-item">${esc(isItemsReq ? "条目清单" : r.item)}</span>
      <span class="req-ttl">${ic("clock")}<span data-countdown></span></span>
    </div>
    <dl class="req-meta">${meta}</dl>
    ${chips ? `<div style="margin-top:8px">${chips}</div>` : ""}
    <div class="req-bar"><i></i></div>
    ${body}
  </article>`;
}

function bindCard(card, list) {
  const rid = card.dataset.rid;
  const r = list.find((x) => x.request_id === rid);
  const alt = card.querySelector(".req-alt");
  card.querySelector(".alt-toggle")?.addEventListener("click", () => {
    alt.hidden = !alt.hidden;
    if (!alt.hidden) { syncAltField(card); card.querySelector(".approve-item").focus(); }
  });
  alt?.querySelectorAll(".seg-btn").forEach((b) => {
    b.onclick = () => {
      alt.querySelectorAll(".seg-btn").forEach((x) => x.classList.remove("active"));
      b.classList.add("active");
    };
  });
  card.querySelector(".approve-btn")?.addEventListener("click", async (e) => {
    const btn = e.currentTarget;
    const item = alt && !alt.hidden ? alt.querySelector(".approve-item").value : "";
    const field = alt && !alt.hidden ? alt.querySelector(".seg-btn.active").dataset.field : (card.dataset.fieldDefault || "secret");
    btn.disabled = true;
    try {
      await call("Approve", rid, item, field);
      toast(`已放行 ${item || r.item}`);
      card.remove();
      refreshPending();
    } catch (e2) { btn.disabled = false; }
  });
  card.querySelector(".deny-btn")?.addEventListener("click", async (e) => {
    e.currentTarget.disabled = true;
    try { await call("Deny", rid); toast("已拒绝"); card.remove(); refreshPending(); }
    catch (e2) { e.currentTarget.disabled = false; }
  });
}

// 与 Android VaultField.defaultFor 同义：有密码放密码，没密码有备注放备注——
// 纯备注条目默认就放备注，批准端不用每次手切。
function fieldDefaultFor(name) {
  const m = state.itemMetas?.get(name);
  if (m && !m.has_value && m.has_note) return "note";
  return "secret";
}

function syncAltField(card) {
  const alt = card.querySelector(".req-alt");
  if (!alt) return;
  const def = fieldDefaultFor(card.querySelector(".approve-item").value);
  alt.querySelectorAll(".seg-btn").forEach((b) => b.classList.toggle("active", b.dataset.field === def));
}

async function fillItemSelects(wrap, list) {
  let metas = [];
  if (state.status?.vault_state === "unlocked") {
    metas = state.items.length ? state.items : await A.Items().catch(() => []);
  }
  state.itemMetas = new Map(metas.map((m) => [m.name, m]));
  wrap.querySelectorAll(".req-card").forEach((card) => {
    const sel = card.querySelector(".approve-item");
    if (!sel) return;
    const want = list.find((r) => r.request_id === card.dataset.rid)?.item;
    sel.innerHTML = [want, ...metas.map((m) => m.name).filter((n) => n !== want)].map((n) => `<option>${esc(n)}</option>`).join("");
    card.dataset.fieldDefault = fieldDefaultFor(want);
    sel.addEventListener("change", () => syncAltField(card));
  });
}

// 渲染差量：列表有焦点（正在选条目/按按钮）时跳过重绘，避免把交互打断。
async function renderPending(freshIds = []) {
  if (!state.status?.paired) return;
  const list = state.pending;
  const wrap = $("pending-list");
  const key = list.map((r) => r.request_id + ":" + r.item).join("|");
  if (!list.length) {
    wrap.innerHTML = `<div class="empty"><div class="glyph">${ic("shield-check")}</div>
      现在没有等待批准的请求<div class="hint">窗口留在托盘；新请求到达时自动弹出并提醒</div></div>`;
    lastPendingKey = key;
    return;
  }
  const focusInside = wrap.contains(document.activeElement);
  if (key !== lastPendingKey && !focusInside) {
    wrap.innerHTML = list.map((r) => pendingCard(r, freshIds)).join("");
    wrap.querySelectorAll(".req-card").forEach((c) => bindCard(c, list));
    await fillItemSelects(wrap, list);
    tickCountdowns();
    // 焦点落在卡片本身而非批准按钮：窗口自动弹出时误按 Enter 不至于直接放密码。
    const fresh = wrap.querySelector(".req-card.fresh");
    if (fresh && freshIds.length) { fresh.setAttribute("tabindex", "-1"); fresh.focus(); }
    lastPendingKey = key;
  }
  tickCountdowns();
}

// 1 秒滴答：只更新倒计时文字和进度条，不动 DOM 结构。
function tickCountdowns() {
  const now = Date.now();
  document.querySelectorAll(".req-card").forEach((card) => {
    const left = Math.round((new Date(card.dataset.exp).getTime() - now) / 1000);
    const total = +card.dataset.ttl || 300;
    const label = card.querySelector("[data-countdown]");
    const bar = card.querySelector(".req-bar");
    const fill = bar?.querySelector("i");
    if (!Number.isFinite(left)) {
      if (label) label.textContent = "--:--";
      return;
    }
    if (label) label.textContent = fmtTtl(left);
    if (fill) fill.style.width = `${Math.max(0, Math.min(100, (left / total) * 100))}%`;
    const urgent = left <= 60;
    bar?.classList.toggle("urgent", urgent);
    card.querySelector(".req-ttl")?.classList.toggle("urgent", urgent);
  });
}
setInterval(tickCountdowns, 1000);

async function refreshPending() {
  if (!state.status?.paired) return;
  let list = [];
  try { list = await A.Pending(); brokerOnline = true; }
  catch (e) { brokerOnline = false; renderStatusLite(); return; }
  const prev = new Set(state.pending.map((r) => r.request_id));
  const freshIds = list.filter((r) => !prev.has(r.request_id)).map((r) => r.request_id);
  state.pending = list;
  const badge = $("pending-badge");
  badge.hidden = !list.length;
  badge.textContent = list.length;
  if (page === "approve") renderPending(freshIds);
}

// 网关状态变化只刷侧栏，不动页面。
function renderStatusLite() { refreshStatus().catch(() => {}); }
