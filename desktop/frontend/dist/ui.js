"use strict";

// 启动期错误可视化：boot 之前没有任何 UI 可依赖，直接铺横幅（发布版也保留，错误必可见）。
window.addEventListener("error", (e) => {
  const el = document.createElement("pre");
  el.style.cssText = "position:fixed;inset:auto 0 0 0;z-index:999;margin:0;padding:10px 14px;background:#b3261e;color:#fff;font:12px/1.5 Consolas;white-space:pre-wrap";
  el.textContent = "JS error: " + (e.message || "?") + " @ " + (e.filename || "") + ":" + (e.lineno || 0) + " / " + (e.error && e.error.stack || "");
  document.body.appendChild(el);
});
window.addEventListener("unhandledrejection", (e) => {
  const el = document.createElement("pre");
  el.style.cssText = "position:fixed;inset:auto 0 40px 0;z-index:999;margin:0;padding:10px 14px;background:#7c4a03;color:#fff;font:12px/1.5 Consolas;white-space:pre-wrap";
  el.textContent = "unhandled rejection: " + String((e.reason && e.reason.message) || e.reason);
  document.body.appendChild(el);
});

// ui.js — 与业务无关的界面原语：Wails 绑定、$、esc、图标、提示条、对话框、时间格式。

const A = window.go && window.go.main && window.go.main.App;

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const ic = (name, cls) => `<svg class="ic${cls ? " " + cls : ""}" aria-hidden="true"><use href="#i-${name}"/></svg>`;

// ---------- 提示条 / 对话框 ----------

function toast(msg, isErr) {
  const root = $("toast-root");
  const t = document.createElement("div");
  t.className = "toast" + (isErr ? " err" : "");
  t.innerHTML = ic(isErr ? "alert-circle" : "check") + esc(msg);
  root.appendChild(t);
  while (root.children.length > 3) root.firstChild.remove();
  setTimeout(() => { t.classList.add("leaving"); setTimeout(() => t.remove(), 220); }, 3200);
}

function call(fn, ...args) {
  return A[fn](...args).catch((e) => {
    const msg = String((e && e.message) || e || "调用失败").replace(/^Error:\s*/, "");
    toast(msg, true);
    throw new Error(msg);
  });
}

// confirm 对话框：resolve(true)=确认。危险按钮放右侧，Esc/遮罩=取消。
function dialog({ title, body, confirmText, danger }) {
  return new Promise((resolve) => {
    const root = $("dialog-root");
    root.innerHTML = `<div class="dialog-mask"><div class="dialog" role="dialog" aria-modal="true" aria-label="${esc(title)}">
      <h2>${esc(title)}</h2>
      <div class="body">${body}</div>
      <div class="actions">
        <button type="button" class="btn quiet" data-act="cancel">取消</button>
        <button type="button" class="btn ${danger ? "danger-primary" : "primary"}" data-act="ok">${esc(confirmText || "确定")}</button>
      </div>
    </div></div>`;
    const done = (v) => { root.innerHTML = ""; document.removeEventListener("keydown", onKey); resolve(v); };
    const onKey = (e) => { if (e.key === "Escape") done(false); if (e.key === "Enter") done(true); };
    document.addEventListener("keydown", onKey);
    root.querySelector('[data-act="cancel"]').onclick = () => done(false);
    root.querySelector('[data-act="ok"]').onclick = () => done(true);
    root.querySelector(".dialog-mask").onclick = (e) => { if (e.target === e.currentTarget) done(false); };
    root.querySelector('[data-act="ok"]').focus();
  });
}

// 带单行输入的对话框：resolve(输入值) 或 resolve(null)=取消。
function dialogPrompt({ title, label, value }) {
  return new Promise((resolve) => {
    const root = $("dialog-root");
    root.innerHTML = `<div class="dialog-mask"><div class="dialog" role="dialog" aria-modal="true" aria-label="${esc(title)}">
      <h2>${esc(title)}</h2>
      <form><label class="field"><span>${esc(label)}</span>
        <input type="text" id="dlg-input" class="mono" value="${esc(value || "")}" />
      </label></form>
      <div class="actions">
        <button type="button" class="btn quiet" data-act="cancel">取消</button>
        <button type="button" class="btn primary" data-act="ok">保存</button>
      </div>
    </div></div>`;
    const input = root.querySelector("#dlg-input");
    input.focus(); input.select();
    const done = (v) => { root.innerHTML = ""; document.removeEventListener("keydown", onKey); resolve(v); };
    const onKey = (e) => { if (e.key === "Escape") done(null); };
    document.addEventListener("keydown", onKey);
    input.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); done(input.value.trim()); } });
    root.querySelector('[data-act="cancel"]').onclick = () => done(null);
    root.querySelector('[data-act="ok"]').onclick = () => done(input.value.trim());
  });
}

// m:ss 倒计时文字。
function fmtTtl(sec) {
  sec = Math.max(0, sec);
  const m = Math.floor(sec / 60), s = sec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

// 相对时间：刚刚 / x 分钟前 / x 小时前 / 日期。
function relTime(iso) {
  if (!iso) return "—";
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 60e3) return "刚刚";
  if (ms < 3600e3) return `${Math.floor(ms / 60e3)} 分钟前`;
  if (ms < 86400e3) return `${Math.floor(ms / 3600e3)} 小时前`;
  return new Date(iso).toLocaleDateString();
}
