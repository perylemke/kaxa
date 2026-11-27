"use strict";

const session = meta("kaxa-session");
const csrf = meta("kaxa-csrf");
const NAME_RULE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const SVG_NS = "http://www.w3.org/2000/svg";
const views = {
  list: document.querySelector("#view-list"),
  detail: document.querySelector("#view-detail"),
  new: document.querySelector("#view-new"),
  terminal: document.querySelector("#view-terminal"),
};
const state = {
  route: { name: "list" },
  providers: [],
  providersLoaded: false,
  sandboxes: [],
  warnings: [],
  listLoaded: false,
  loadedAt: null,
  filter: { provider: "", query: "" },
  detail: null,
  destroyArmed: false,
  fresh: null,
  creating: false,
  templateFor: "",
  templates: {},
  runningStep: null,
  startedAt: 0,
  repos: [],
  picker: { path: "", parent: "" },
  timer: null,
  focusPending: false,
  term: null,
};

const navOn = "rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm text-kx-on-button";
const navOff = "rounded-lg border border-kx-line bg-kx-surface-2 px-3.5 py-2 text-sm text-kx-text hover:bg-kx-hover";
const primaryClass = "justify-self-start rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm font-medium text-kx-on-button disabled:cursor-not-allowed disabled:opacity-45";
const quietClass = "rounded-lg border border-kx-line bg-kx-surface-2 px-3.5 py-2 text-sm text-kx-text hover:bg-kx-hover disabled:cursor-not-allowed disabled:opacity-45";
const dangerClass = "justify-self-start rounded-lg border border-transparent bg-kx-danger-bg px-3.5 py-2 text-sm font-medium text-kx-danger disabled:cursor-not-allowed disabled:opacity-45";
const termTheme = { background: "#07111f", foreground: "#e7eef8", cursor: "#7eb6ff", selectionBackground: "#183056" };
const termPageClass = "h-screen overflow-hidden bg-kx-bg";
const termHostClass = "fixed inset-0 bg-kx-bg p-1";
const pageClass = document.body.className;
const codeBoxClass = "block min-w-0 flex-1 overflow-x-auto rounded-lg border border-kx-line bg-kx-bg px-3 py-2 font-mono text-sm";
let xtermReady = null;

const linkButtonClass = "inline-flex w-fit items-center rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm font-medium text-kx-on-button";
const textButtonClass = "w-fit text-sm text-kx-accent underline decoration-kx-accent/40 underline-offset-2 disabled:opacity-45";
const linkClass = "break-all text-sm text-kx-accent underline decoration-kx-accent/40 underline-offset-2";
const monoClass = "break-all font-mono text-sm";
const headingClass = "text-xl tracking-tight outline-none";
const mutedClass = "text-sm text-kx-muted";
const noticeClass = "break-words rounded-lg border border-kx-button bg-kx-button/15 px-3.5 py-3 text-sm";
const errorBoxClass = "break-words rounded-lg border border-kx-danger/40 bg-kx-danger-bg px-3.5 py-3 text-sm text-kx-danger";
const livePillClass = "rounded-full bg-kx-accent/15 px-2.5 py-0.5 text-xs font-medium text-kx-accent";
const idlePillClass = "rounded-full bg-kx-surface-2 px-2.5 py-0.5 text-xs font-medium text-kx-muted";
const tagClass = "rounded bg-kx-surface-2 px-1.5 py-0.5 text-xs text-kx-muted";
const rowClass = "flex items-center gap-4 px-1 py-3 hover:bg-kx-hover";
const fieldClass = "w-full rounded-lg border border-kx-line bg-kx-surface px-3 py-2.5 placeholder:text-kx-muted";
const panelClass = "grid gap-3 rounded-lg border border-kx-line bg-kx-surface p-4";
const providerCardClass = "flex cursor-pointer items-start gap-3 rounded-lg border border-kx-line bg-kx-surface p-3 has-[:checked]:border-kx-accent has-[:checked]:bg-kx-accent/10 has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50";
const segmentOnClass = "rounded-md bg-kx-button px-3 py-1.5 text-sm text-kx-on-button disabled:cursor-not-allowed";
const segmentOffClass = "rounded-md px-3 py-1.5 text-sm text-kx-muted hover:text-kx-text disabled:cursor-not-allowed disabled:opacity-45";

window.addEventListener("hashchange", route);
document.querySelector("#create-form").addEventListener("submit", onCreate);
document.querySelector("#create-name").addEventListener("input", () => checkName(false));
document.querySelector("#create-sync").addEventListener("change", syncCreateFields);
document.querySelector("#create-api").addEventListener("change", syncCreateFields);
document.querySelector("#create-template").addEventListener("change", syncTemplateOther);
document.querySelector("#create-template-other").addEventListener("input", () => checkTemplate(true));
for (const id of ["create-cpu", "create-memory"]) {
  document.querySelector("#" + id).addEventListener("change", syncOther);
}
for (const id of ["create-cpu-other", "create-memory-other", "create-disk"]) {
  document.querySelector("#" + id).addEventListener("input", () => checkCompute(true));
}
document.querySelector("#repo-open").addEventListener("click", () => openPicker(""));
document.querySelector("#picker-close").addEventListener("click", closePicker);
document.querySelector("#picker-up").addEventListener("click", () => openPicker(state.picker.parent));
document.querySelector("#picker-here").addEventListener("click", () => addRepo(state.picker.path));
document.querySelector("#repo-picker").addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    event.stopPropagation();
    closePicker();
    document.querySelector("#repo-open").focus();
  }
});

function meta(name) {
  const el = document.querySelector(`meta[name="${name}"]`);
  return el ? el.content : "";
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) {
    node.className = className;
  }
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

function icon(paths, className) {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("class", className);
  for (const d of paths) {
    const path = document.createElementNS(SVG_NS, "path");
    path.setAttribute("d", d);
    svg.append(path);
  }
  return svg;
}

function chevron() {
  return icon(["m9 18 6-6-6-6"], "size-4 shrink-0 text-kx-muted");
}

function stepIcon(status) {
  if (status === "done") {
    return icon(["M20 6 9 17l-5-5"], "size-4 shrink-0 text-kx-accent");
  }
  if (status === "failed") {
    return icon(["M18 6 6 18", "M6 6l12 12"], "size-4 shrink-0 text-kx-danger");
  }
  return icon(["M21 12a9 9 0 1 1-6.2-8.6"], "size-4 shrink-0 animate-spin text-kx-accent");
}

function apiHeaders(method) {
  const headers = { "X-Kaxa-Session": session };
  if (method !== "GET") {
    headers["X-Kaxa-CSRF"] = csrf;
    headers["Content-Type"] = "application/json";
  }
  return headers;
}

async function readError(res) {
  let body = {};
  try {
    body = JSON.parse(await res.text());
  } catch {
    body = {};
  }
  const err = new Error(body.error || "request failed");
  err.hint = body.hint || "";
  return err;
}

async function api(path, options = {}) {
  const method = options.method || "GET";
  const init = { method, headers: apiHeaders(method) };
  if (method !== "GET") {
    init.body = JSON.stringify(options.body || {});
  }
  const res = await fetch(path, init);
  if (!res.ok) {
    throw await readError(res);
  }
  const text = await res.text();
  return text ? JSON.parse(text) : {};
}

function messageOf(err) {
  if (!err) {
    return "request failed";
  }
  return err.hint ? `${err.message} ${err.hint}` : err.message || "request failed";
}

function showError(err) {
  const notice = document.querySelector("#notice");
  notice.hidden = false;
  notice.className = noticeClass;
  notice.textContent = messageOf(err);
}

function clearNotice() {
  const notice = document.querySelector("#notice");
  notice.hidden = true;
  notice.textContent = "";
}

let toastTimer = null;

function toast(message, ms = 5000) {
  const box = document.querySelector("#toast");
  document.querySelector("#live").textContent = message;
  box.textContent = message;
  box.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    box.hidden = true;
  }, ms);
}

function settle(root) {
  if (!state.focusPending) {
    return;
  }
  state.focusPending = false;
  const heading = root.querySelector("h1");
  if (heading) {
    heading.setAttribute("tabindex", "-1");
    heading.focus({ preventScroll: true });
  }
}

function parseRoute() {
  const parts = location.hash.replace(/^#/, "").split("/").filter(Boolean);
  try {
    if (parts.length === 1 && parts[0] === "new") {
      return { name: "new" };
    }
    if (parts.length === 3 && parts[0] === "terminal") {
      return { name: "terminal", provider: decodeURIComponent(parts[1]), id: decodeURIComponent(parts[2]) };
    }
    if (parts.length === 3 && parts[0] === "sandboxes") {
      return { name: "detail", provider: decodeURIComponent(parts[1]), id: decodeURIComponent(parts[2]) };
    }
  } catch {
    return { name: "list" };
  }
  return { name: "list" };
}

function route() {
  const next = parseRoute();
  state.route = next;
  state.focusPending = true;
  state.destroyArmed = false;
  closeTerminal();
  clearNotice();
  if (next.name !== "list") {
    document.querySelector("#warnings").replaceChildren();
  }
  showView(next);
  if (next.name === "list") {
    loadList(false).catch(showError);
  } else if (next.name === "new") {
    openCreate().catch(showError);
  } else if (next.name === "terminal") {
    openTerminalTab(next.provider, next.id);
  } else {
    openDetail(next.provider, next.id);
  }
}

function showView(next) {
  for (const [key, view] of Object.entries(views)) {
    view.hidden = key !== next.name;
  }
  const onList = next.name === "list" || next.name === "detail";
  document.querySelector("#nav-list").className = onList ? navOn : navOff;
  document.querySelector("#nav-new").className = next.name === "new" ? navOn : navOff;
  setCurrent("#nav-list", onList);
  setCurrent("#nav-new", next.name === "new");
  const titles = { list: "Sandboxes", new: "New sandbox", detail: next.id, terminal: `${next.id} · terminal` };
  document.title = `${titles[next.name]} · kaxa`;
  const full = next.name === "terminal";
  document.body.className = full ? termPageClass : pageClass;
  document.querySelector("header").hidden = full;
  document.querySelector("#main").className = full ? "outline-none" : "grid gap-6 outline-none";
}

function setCurrent(selector, on) {
  const link = document.querySelector(selector);
  if (on) {
    link.setAttribute("aria-current", "page");
  } else {
    link.removeAttribute("aria-current");
  }
}

function sandboxHash(provider, id) {
  return `#/sandboxes/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`;
}

function sandboxPath(detail, action) {
  const base = `/api/sandboxes/${encodeURIComponent(detail.provider)}/${encodeURIComponent(detail.id)}`;
  return action ? `${base}/${action}` : base;
}

/* List */

async function loadProviders() {
  const providers = await api("/api/providers");
  state.providers = providers.providers || [];
  state.providersLoaded = true;
}

async function loadList(manual) {
  if (!state.listLoaded) {
    renderListSkeleton();
  }
  const listed = api("/api/sandboxes").catch((err) => {
    showError(err);
    return { sandboxes: [], warnings: [] };
  });
  const [, sandboxes] = await Promise.all([loadProviders(), listed]);
  state.sandboxes = sandboxes.sandboxes || [];
  state.warnings = sandboxes.warnings || [];
  state.listLoaded = true;
  state.loadedAt = new Date();
  if (state.route.name !== "list") {
    return;
  }
  renderWarnings();
  renderList();
  if (manual) {
    toast("Sandbox list updated");
  }
}

function renderListSkeleton() {
  const root = views.list;
  root.replaceChildren();
  root.className = "grid gap-4";
  const title = el("h1", headingClass, "Sandboxes");
  title.id = "list-title";
  root.append(title);
  for (let i = 0; i < 3; i++) {
    root.append(el("div", "h-12 animate-pulse rounded-lg bg-kx-surface", ""));
  }
  settle(root);
}

function renderWarnings() {
  const root = document.querySelector("#warnings");
  root.replaceChildren();
  for (const warning of state.warnings) {
    const row = el("div", `${noticeClass} flex items-start justify-between gap-3`);
    row.append(el("p", "min-w-0", warning));
    const close = el("button", textButtonClass, "Dismiss");
    close.type = "button";
    close.addEventListener("click", () => row.remove());
    row.append(close);
    root.append(row);
  }
}

function renderList() {
  const root = views.list;
  root.replaceChildren();
  root.className = "grid gap-4";
  root.append(listHeader());
  if (state.sandboxes.length === 0) {
    root.append(emptyState());
    settle(root);
    return;
  }
  if (needsFilter()) {
    root.append(listFilters());
  }
  const rows = el("div", "divide-y divide-kx-line");
  rows.id = "rows";
  root.append(rows);
  renderRows();
  settle(root);
}

function listHeader() {
  const head = el("div", "flex flex-wrap items-baseline justify-between gap-3");
  const left = el("div", "flex items-baseline gap-3");
  const title = el("h1", headingClass, "Sandboxes");
  title.id = "list-title";
  left.append(title);
  const count = state.sandboxes.length;
  if (count > 0) {
    left.append(el("p", mutedClass, count === 1 ? "1 sandbox" : `${count} sandboxes`));
  }
  const right = el("div", "flex items-center gap-3");
  if (state.loadedAt) {
    right.append(el("p", "text-xs text-kx-muted", `Updated ${state.loadedAt.toLocaleTimeString()}`));
  }
  const refresh = el("button", quietClass, "Refresh");
  refresh.type = "button";
  refresh.addEventListener("click", () => {
    refresh.disabled = true;
    refresh.textContent = "Refreshing…";
    loadList(true).catch(showError).finally(() => {
      refresh.disabled = false;
      refresh.textContent = "Refresh";
    });
  });
  right.append(refresh);
  head.append(left, right);
  return head;
}

function emptyState() {
  const box = el("div", `${panelClass} justify-items-start`);
  box.append(el("p", "font-medium", "No sandboxes yet."));
  box.append(el("p", mutedClass, "A sandbox is an isolated environment on one of your providers. Create one to run builds, tests, and services away from this machine."));
  const link = el("a", linkButtonClass, "Create a sandbox");
  link.href = "#/new";
  box.append(link);
  return box;
}

function needsFilter() {
  const names = new Set(state.sandboxes.map((box) => box.provider));
  return names.size > 1 || state.sandboxes.length > 8;
}

function listFilters() {
  const bar = el("div", "flex flex-wrap gap-2");
  const search = el("input", `${fieldClass} min-w-40 flex-1`);
  search.type = "search";
  search.placeholder = "Search by name";
  search.setAttribute("aria-label", "Search sandboxes by name");
  search.value = state.filter.query;
  search.addEventListener("input", () => {
    state.filter.query = search.value.trim().toLowerCase();
    renderRows();
  });
  const select = el("select", `${fieldClass} w-auto`);
  select.setAttribute("aria-label", "Filter by provider");
  select.append(new Option("All providers", ""));
  for (const name of [...new Set(state.sandboxes.map((box) => box.provider))].sort()) {
    select.append(new Option(name, name));
  }
  select.value = state.filter.provider;
  select.addEventListener("change", () => {
    state.filter.provider = select.value;
    renderRows();
  });
  bar.append(search, select);
  return bar;
}

function renderRows() {
  const root = document.querySelector("#rows");
  if (!root) {
    return;
  }
  root.replaceChildren();
  const shown = state.sandboxes.filter((box) => {
    const byProvider = !state.filter.provider || box.provider === state.filter.provider;
    return byProvider && (box.id || "").toLowerCase().includes(state.filter.query);
  });
  if (shown.length === 0) {
    root.append(el("p", `${mutedClass} py-3`, "No sandboxes match."));
    return;
  }
  for (const box of shown) {
    root.append(sandboxRow(box));
  }
}

function sandboxRow(box) {
  const link = el("a", rowClass);
  link.href = sandboxHash(box.provider, box.id);
  link.append(el("span", "min-w-0 flex-1 truncate font-medium", box.id || ""));
  link.append(el("span", mutedClass, box.provider || ""));
  link.append(statusPill(box.status));
  link.append(chevron());
  return link;
}

function statusPill(status) {
  const live = status === "running" || status === "warm";
  return el("span", live ? livePillClass : idlePillClass, status || "unknown");
}

/* Detail */

let actionError = null;

async function openDetail(provider, id) {
  renderDetailLoading(provider, id);
  try {
    state.detail = await api(sandboxPath({ provider, id }));
  } catch (err) {
    if (isDetailRoute(provider, id)) {
      renderDetailError(provider, id, err);
    }
    return;
  }
  if (isDetailRoute(provider, id)) {
    renderDetail();
  }
}

function isDetailRoute(provider, id) {
  return state.route.name === "detail" && state.route.provider === provider && state.route.id === id;
}

function backLink(provider, id) {
  const nav = el("nav", "flex flex-wrap items-center gap-1.5 text-sm text-kx-muted");
  nav.setAttribute("aria-label", "Breadcrumb");
  const all = el("a", "text-kx-accent underline decoration-kx-accent/40 underline-offset-2", "Sandboxes");
  all.href = "#/";
  nav.append(all, el("span", "", "/"), el("span", "", provider), el("span", "", "/"));
  const current = el("span", "text-kx-text", id);
  current.setAttribute("aria-current", "page");
  nav.append(current);
  return nav;
}

function renderDetailLoading(provider, id) {
  const root = views.detail;
  root.replaceChildren();
  root.className = "grid gap-4";
  root.append(backLink(provider, id));
  root.append(el("h1", headingClass, id));
  root.append(el("div", "h-24 animate-pulse rounded-lg bg-kx-surface", ""));
  settle(root);
}

function renderDetailError(provider, id, err) {
  const root = views.detail;
  root.replaceChildren();
  root.className = "grid gap-4";
  root.append(backLink(provider, id));
  root.append(el("h1", headingClass, "Sandbox unavailable"));
  root.append(el("p", errorBoxClass, messageOf(err)));
  const retry = el("button", quietClass, "Try again");
  retry.type = "button";
  retry.addEventListener("click", () => openDetail(provider, id));
  const back = el("a", textButtonClass, "Back to sandboxes");
  back.href = "#/";
  const row = el("div", "flex flex-wrap items-center gap-3");
  row.append(retry, back);
  root.append(row);
  settle(root);
}

function renderDetail() {
  const detail = state.detail;
  const root = views.detail;
  const caps = detail.capabilities || {};
  root.replaceChildren();
  root.className = "grid gap-5";
  root.append(backLink(detail.provider, detail.id));
  root.append(detailHeader(detail));
  if (state.fresh && state.fresh.provider === detail.provider && state.fresh.id === detail.id) {
    root.append(freshPanel(state.fresh));
  }
  actionError = el("p", errorBoxClass);
  actionError.setAttribute("role", "alert");
  actionError.hidden = true;
  root.append(actionError);
  if (detail.url) {
    root.append(urlPanel(detail, caps));
  }
  const fields = detailFields(detail);
  if (fields) {
    root.append(fields);
  }
  root.append(shellPanel(detail));
  root.append(destroyZone(detail));
  settle(root);
}

function detailHeader(detail) {
  const caps = detail.capabilities || {};
  const head = el("div", "flex flex-wrap items-center justify-between gap-3");
  const left = el("div", "flex flex-wrap items-center gap-3");
  left.append(el("h1", headingClass, detail.id || ""), statusPill(detail.status));
  head.append(left);
  const actions = el("div", "flex flex-wrap gap-2");
  if (caps.restart) {
    actions.append(actionButton("Restart", "Restarting…", () => restartLike("restart", "Restarted")));
  }
  if (caps.upgrade) {
    actions.append(actionButton("Upgrade", "Upgrading…", () => restartLike("upgrade", "Upgraded")));
  }
  if (actions.childElementCount > 0) {
    head.append(actions);
  }
  return head;
}

function actionButton(label, busyLabel, task, className) {
  const button = el("button", className || quietClass, label);
  button.type = "button";
  button.addEventListener("click", () => perform(button, busyLabel, task));
  return button;
}

async function perform(button, busyLabel, task) {
  const root = views.detail;
  const buttons = [...root.querySelectorAll("button")];
  const label = button.textContent;
  buttons.forEach((b) => {
    b.disabled = true;
  });
  button.textContent = busyLabel;
  hideActionError();
  try {
    await task();
  } catch (err) {
    showActionError(err);
  } finally {
    buttons.forEach((b) => {
      b.disabled = false;
    });
    button.textContent = label;
  }
}

function showActionError(err) {
  if (actionError) {
    actionError.textContent = messageOf(err);
    actionError.hidden = false;
  } else {
    showError(err);
  }
}

function hideActionError() {
  if (actionError) {
    actionError.hidden = true;
    actionError.textContent = "";
  }
}

async function restartLike(action, done) {
  const detail = state.detail;
  await api(sandboxPath(detail, action), { method: "POST", body: {} });
  state.detail = await api(sandboxPath(detail));
  renderDetail();
  toast(`${done} ${detail.id}`);
}

function urlPanel(detail, caps) {
  const panel = el("div", panelClass);
  panel.append(el("p", "text-sm font-medium", "URL"));
  const row = el("div", "flex flex-wrap items-center gap-3");
  const link = el("a", linkClass, detail.url);
  link.href = detail.url;
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  row.append(link);
  const copy = el("button", quietClass, "Copy");
  copy.type = "button";
  copy.addEventListener("click", () => copyText(detail.url, copy));
  row.append(copy);
  panel.append(row);
  if (caps.public_url) {
    panel.append(authControl(detail));
  }
  return panel;
}

function authControl(detail) {
  const wrap = el("div", "grid gap-2");
  wrap.append(el("p", mutedClass, "Who can open this URL"));
  const group = el("div", "flex w-fit gap-1 rounded-lg border border-kx-line bg-kx-bg p-1");
  group.setAttribute("role", "group");
  group.setAttribute("aria-label", "URL access");
  for (const [value, label] of [["public", "Public"], ["sprite", "Account"]]) {
    const active = detail.auth === value;
    const button = el("button", active ? segmentOnClass : segmentOffClass, label);
    button.type = "button";
    button.setAttribute("aria-pressed", String(active));
    if (!active) {
      button.addEventListener("click", () => perform(button, "Saving…", () => setAuth(value)));
    }
    group.append(button);
  }
  wrap.append(group);
  const hint = detail.auth === "public" ? "Anyone with the link can open it." : "Only your provider account can open it.";
  wrap.append(el("p", "text-xs text-kx-muted", hint));
  return wrap;
}

async function setAuth(auth) {
  const detail = state.detail;
  state.detail = await api(sandboxPath(detail, "url-auth"), { method: "POST", body: { auth } });
  renderDetail();
  toast(auth === "public" ? "URL is now public" : "URL is now account-only");
}

function detailFields(detail) {
  const dl = el("dl", "grid grid-cols-[minmax(7.5rem,11rem)_1fr] gap-x-4 gap-y-2 rounded-lg border border-kx-line bg-kx-surface p-4");
  const add = (label, value) => {
    dl.append(el("dt", mutedClass, label), el("dd", "break-words text-sm", value));
  };
  for (const field of detail.fields || []) {
    add(field.label, field.text || field.at || "");
  }
  if (detail.created_at) {
    add("Created", detail.created_at);
  }
  if (detail.updated_at) {
    add("Updated", detail.updated_at);
  }
  return dl.childElementCount > 0 ? dl : null;
}

function freshPanel(fresh) {
  const panel = el("div", "grid gap-3 rounded-lg border border-kx-accent/40 bg-kx-accent/10 p-4");
  const head = el("div", "flex items-center justify-between gap-3");
  head.append(el("p", "font-medium", `Created ${fresh.id}`));
  const dismiss = el("button", textButtonClass, "Dismiss");
  dismiss.type = "button";
  dismiss.addEventListener("click", () => {
    state.fresh = null;
    renderDetail();
  });
  head.append(dismiss);
  panel.append(head);
  if (fresh.work_dir) {
    panel.append(el("p", mutedClass, "Working directory"));
    panel.append(el("p", monoClass, fresh.work_dir));
  }
  if (fresh.preview_token) {
    panel.append(el("p", mutedClass, "Preview token (shown once)"));
    const row = el("div", "flex flex-wrap items-center gap-3");
    row.append(el("p", monoClass, fresh.preview_token));
    const copy = el("button", quietClass, "Copy");
    copy.type = "button";
    copy.addEventListener("click", () => copyText(fresh.preview_token, copy));
    row.append(copy);
    panel.append(row);
    panel.append(el("p", "text-xs text-kx-muted", "Copy it now. kaxa ui does not show it again."));
  }
  for (const command of fresh.skipped_commands || []) {
    panel.append(el("p", mutedClass, `Skipped: ${command.join(" ")}`));
  }
  return panel;
}

function destroyZone(detail) {
  const zone = el("div", "mt-4 grid gap-3 border-t border-kx-line pt-6");
  zone.append(el("h2", "text-sm font-medium", "Danger zone"));
  if (!state.destroyArmed) {
    zone.append(el("p", mutedClass, "Permanently delete this sandbox and everything in it."));
    const arm = el("button", dangerClass, "Destroy");
    arm.type = "button";
    arm.addEventListener("click", () => {
      state.destroyArmed = true;
      renderDetail();
      const cancel = views.detail.querySelector("[data-cancel-destroy]");
      if (cancel) {
        cancel.focus();
      }
    });
    zone.append(arm);
    return zone;
  }
  zone.append(el("p", "text-sm", `Destroy ${detail.provider}/${detail.id}? This cannot be undone.`));
  const row = el("div", "flex flex-wrap gap-2");
  const cancel = el("button", quietClass, "Cancel");
  cancel.type = "button";
  cancel.setAttribute("data-cancel-destroy", "");
  cancel.addEventListener("click", () => {
    state.destroyArmed = false;
    renderDetail();
  });
  row.append(cancel, actionButton(`Destroy ${detail.id}`, "Destroying…", onDestroy, dangerClass));
  zone.append(row);
  return zone;
}

async function onDestroy() {
  const detail = state.detail;
  const result = await api(sandboxPath(detail, "destroy"), { method: "POST", body: { confirm: detail.id } });
  state.detail = null;
  state.destroyArmed = false;
  if (result && result.notice) {
    toast(`Destroyed ${detail.id}: ${result.notice}`, 12000);
  } else {
    toast(`Destroyed ${detail.id}`);
  }
  location.hash = "#/";
}

async function copyText(text, button) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    toast("Copy failed. Select the text and copy it by hand.");
    return;
  }
  const label = button.textContent;
  button.textContent = "Copied";
  setTimeout(() => {
    button.textContent = label;
  }, 1800);
}

/* Create */

async function openCreate() {
  if (!state.providersLoaded) {
    await loadProviders();
  }
  fillProviders();
  settle(views.new);
}

function selectedProvider() {
  const input = document.querySelector('input[name="provider"]:checked');
  return input ? state.providers.find((provider) => provider.name === input.value) || null : null;
}

function fillProviders() {
  const root = document.querySelector("#provider-options");
  const chosen = selectedProvider();
  const pick = [
    state.providers.find((p) => chosen && p.name === chosen.name && p.available),
    state.providers.find((p) => p.default && p.available),
    state.providers.find((p) => p.available),
  ].find(Boolean);
  root.replaceChildren();
  for (const provider of state.providers) {
    root.append(providerOption(provider, Boolean(pick) && pick.name === provider.name));
  }
  syncCreateFields();
}

function providerOption(provider, checked) {
  const label = el("label", providerCardClass);
  const input = el("input", "mt-1 size-4 accent-kx-button");
  input.type = "radio";
  input.name = "provider";
  input.value = provider.name;
  input.checked = checked;
  input.disabled = !provider.available;
  input.addEventListener("change", syncCreateFields);
  const body = el("div", "grid gap-1.5");
  body.append(el("span", "text-sm font-medium", provider.name));
  if (!provider.available) {
    body.append(el("span", "text-xs text-kx-muted", "Not configured"));
  } else {
    const tags = el("span", "flex flex-wrap gap-1");
    for (const tag of providerTags(provider.capabilities || {})) {
      tags.append(el("span", tagClass, tag));
    }
    body.append(tags);
  }
  label.append(input, body);
  return label;
}

function providerTags(caps) {
  const tags = [];
  if (caps.running) {
    tags.push("Keep running");
  }
  if (caps.sync) {
    tags.push("Repo sync");
  }
  if (caps.api) {
    tags.push("HTTP service");
  }
  if (caps.public_url || caps.public_preview) {
    tags.push("Public URL");
  }
  if (caps.cpu || caps.memory) {
    tags.push("CPU and memory");
  }
  if (caps.template) {
    tags.push("Custom image");
  }
  return tags;
}

const OTHER = "other";

function computeEl(id) {
  return document.querySelector("#" + id);
}

function memoryLabel(mib) {
  return mib >= 1024 && mib % 1024 === 0 ? mib / 1024 + " GiB (" + mib + " MiB)" : mib + " MiB";
}

function fillSelect(select, choices, withOther) {
  select.replaceChildren();
  const rows = [["", "Provider default"], ...choices];
  if (withOther) {
    rows.push([OTHER, "Other"]);
  }
  for (const [value, label] of rows) {
    const option = el("option", "", label);
    option.value = value;
    select.append(option);
  }
}

// syncCompute shows the resource fields the selected provider accepts. The
// choices are refilled only when the provider changes, so a pick survives the
// other toggles of the form.
function syncCompute(provider, caps) {
  const options = provider && provider.available ? provider.compute || {} : {};
  const sizes = options.sizes || [];
  const cpus = options.cpus || [];
  const memory = options.memory_mib || [];
  const name = provider ? provider.name : "";
  if (state.computeFor !== name) {
    state.computeFor = name;
    fillSelect(computeEl("create-size"), sizes.map((size) => [size.key, size.label]), false);
    fillSelect(computeEl("create-cpu"), cpus.map((cpu) => [String(cpu), cpu + (cpu === 1 ? " core" : " cores")]), Boolean(options.custom_cpu));
    fillSelect(computeEl("create-memory"), memory.map((mib) => [String(mib), memoryLabel(mib)]), Boolean(options.custom_memory));
    for (const id of ["create-cpu-other", "create-memory-other", "create-disk"]) {
      computeEl(id).value = "";
    }
    syncOther();
  }
  const showSize = sizes.length > 0;
  const showCPU = cpus.length > 0 || Boolean(options.custom_cpu);
  const showMemory = memory.length > 0 || Boolean(options.custom_memory);
  const showDisk = Boolean(caps.disk_percent);
  computeEl("size-field").hidden = !showSize;
  computeEl("cpu-field").hidden = !showCPU;
  computeEl("memory-field").hidden = !showMemory;
  computeEl("disk-field").hidden = !showDisk;
  computeEl("compute-fields").hidden = !(showSize || showCPU || showMemory || showDisk);
}

function syncOther() {
  computeEl("create-cpu-other").hidden = computeEl("create-cpu").value !== OTHER;
  computeEl("create-memory-other").hidden = computeEl("create-memory").value !== OTHER;
  checkCompute(false);
}

function computeFieldHidden(id) {
  return computeEl(id).closest("[id$='-field']").hidden || computeEl("compute-fields").hidden;
}

function setComputeProblem(input, helpID, message, show) {
  const help = computeEl(helpID);
  const bad = Boolean(message) && show;
  input.setAttribute("aria-invalid", bad ? "true" : "false");
  help.hidden = !bad && helpID !== "disk-help";
  if (helpID !== "disk-help") {
    help.textContent = bad ? message : "";
    help.className = "text-xs text-kx-danger";
  } else {
    help.className = bad ? "text-xs text-kx-danger" : "text-xs text-kx-muted";
  }
}

function numberProblem(select, input, hidden, whole, label) {
  if (hidden || select.value !== OTHER) {
    return "";
  }
  const raw = input.value.trim();
  const value = Number(raw);
  if (raw === "" || !Number.isFinite(value) || value <= 0) {
    return "Enter " + label + " greater than zero.";
  }
  if (whole && !Number.isInteger(value)) {
    return "Enter a whole number.";
  }
  return "";
}

function diskProblem() {
  const raw = computeEl("create-disk").value.trim();
  if (computeFieldHidden("create-disk") || raw === "") {
    return "";
  }
  const value = Number(raw);
  return Number.isInteger(value) && value >= 10 && value <= 95 ? "" : "Use a whole number from 10 to 95.";
}

// checkCompute validates the custom values; show reveals the messages.
function checkCompute(show) {
  const cpu = numberProblem(computeEl("create-cpu"), computeEl("create-cpu-other"), computeFieldHidden("create-cpu"), false, "the CPU cores");
  const memory = numberProblem(computeEl("create-memory"), computeEl("create-memory-other"), computeFieldHidden("create-memory"), true, "the memory in MiB");
  const disk = diskProblem();
  setComputeProblem(computeEl("create-cpu-other"), "cpu-help", cpu, show);
  setComputeProblem(computeEl("create-memory-other"), "memory-help", memory, show);
  setComputeProblem(computeEl("create-disk"), "disk-help", disk, show);
  if (cpu) {
    return computeEl("create-cpu-other");
  }
  if (memory) {
    return computeEl("create-memory-other");
  }
  return disk ? computeEl("create-disk") : null;
}

/* Image */

const imageRef = /^[A-Za-z0-9][A-Za-z0-9._:/@-]*$/;

function templateEl(id) {
  return document.querySelector("#" + id);
}

// syncTemplate shows the image field for a provider that starts from an image.
// The catalog is fetched once per provider; without one, the field stays a text input.
function syncTemplate(provider, caps) {
  const name = provider && provider.available && caps.template ? provider.name : "";
  templateEl("template-fields").hidden = name === "";
  if (state.templateFor === name) {
    return;
  }
  state.templateFor = name;
  templateEl("create-template-other").value = "";
  renderTemplates([]);
  if (name === "") {
    return;
  }
  templateEl("template-help").textContent = "Loading images…";
  loadTemplates(name).then((list) => {
    if (state.templateFor === name) {
      renderTemplates(list);
    }
  });
}

async function loadTemplates(name) {
  if (!state.templates[name]) {
    try {
      const out = await api(`/api/providers/${encodeURIComponent(name)}/templates`);
      state.templates[name] = out.templates || [];
    } catch {
      state.templates[name] = [];
    }
  }
  return state.templates[name];
}

const templateHelp = "Empty keeps the provider default. Use an image reference such as registry/name:tag, or a snapshot ID on Vercel.";

function renderTemplates(list) {
  const select = templateEl("create-template");
  templateEl("template-help").textContent = templateHelp;
  select.hidden = list.length === 0;
  templateEl("template-label").htmlFor = list.length === 0 ? "create-template-other" : "create-template";
  if (list.length > 0) {
    fillSelect(select, list.map((item) => [item.name, item.label || item.name]), true);
  }
  syncTemplateOther();
}

function syncTemplateOther() {
  const select = templateEl("create-template");
  templateEl("create-template-other").hidden = !select.hidden && select.value !== OTHER;
  checkTemplate(false);
}

// templateValue is the image reference the form names, empty for the provider default.
function templateValue() {
  if (templateEl("template-fields").hidden) {
    return "";
  }
  const select = templateEl("create-template");
  if (select.hidden || select.value === OTHER) {
    return templateEl("create-template-other").value.trim();
  }
  return select.value;
}

// checkTemplate flags characters an image reference cannot hold; the provider checks the rest.
function checkTemplate(show) {
  const input = templateEl("create-template-other");
  const raw = templateValue();
  const bad = !input.hidden && raw !== "" && (!imageRef.test(raw) || raw.includes(".."));
  input.setAttribute("aria-invalid", bad && show ? "true" : "false");
  templateEl("template-help").textContent = bad && show ? "Use letters, digits, and . _ : / @ - in an image reference." : templateHelp;
  templateEl("template-help").className = bad && show ? "text-xs text-kx-danger" : "text-xs text-kx-muted";
  return bad ? input : null;
}

function computeBody() {
  const out = {};
  if (computeEl("compute-fields").hidden) {
    return out;
  }
  const provider = selectedProvider();
  const sizes = ((provider && provider.compute) || {}).sizes || [];
  const size = sizes.find((item) => item.key === computeEl("create-size").value);
  if (!computeEl("size-field").hidden && size) {
    out.cpu = size.cpu;
    out.memory_mib = size.memory_mib;
  }
  const cpu = computeEl("create-cpu").value;
  if (!computeEl("cpu-field").hidden && cpu !== "") {
    out.cpu = Number(cpu === OTHER ? computeEl("create-cpu-other").value : cpu);
  }
  const memory = computeEl("create-memory").value;
  if (!computeEl("memory-field").hidden && memory !== "") {
    out.memory_mib = Number(memory === OTHER ? computeEl("create-memory-other").value : memory);
  }
  const disk = computeEl("create-disk").value.trim();
  if (!computeEl("disk-field").hidden && disk !== "") {
    out.disk_percent = Number(disk);
  }
  return out;
}

function syncCreateFields() {
  const provider = selectedProvider();
  const caps = provider && provider.available ? provider.capabilities || {} : {};
  const syncOn = document.querySelector("#create-sync").checked;
  const apiOn = document.querySelector("#create-api").checked;
  setCap("running", Boolean(caps.running));
  setCap("sync", Boolean(caps.sync));
  setCap("api", Boolean(caps.api));
  setCap("public", Boolean(caps.public_url) || (Boolean(caps.public_preview) && apiOn));
  document.querySelector("#sync-extra").hidden = !caps.sync || !syncOn;
  document.querySelector("#api-extra").hidden = !caps.api || !apiOn;
  syncCompute(provider, caps);
  syncTemplate(provider, caps);
  document.querySelector("#create-submit").disabled = !provider || !provider.available || state.creating;
}

/* Repository picker */

function baseName(path) {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.length ? parts[parts.length - 1] : path;
}

function renderRepos() {
  const list = document.querySelector("#repo-list");
  list.replaceChildren();
  for (const path of state.repos) {
    const item = el("li", "flex items-center justify-between gap-3 rounded-lg border border-kx-line bg-kx-surface px-3 py-2");
    const text = el("div", "min-w-0");
    text.append(el("p", "truncate text-sm font-medium", baseName(path)), el("p", "truncate font-mono text-xs text-kx-muted", path));
    const remove = el("button", textButtonClass, "Remove");
    remove.type = "button";
    remove.addEventListener("click", () => {
      state.repos = state.repos.filter((repo) => repo !== path);
      renderRepos();
    });
    item.append(text, remove);
    list.append(item);
  }
  document.querySelector("#repo-empty").hidden = state.repos.length > 0;
}

function addRepo(path) {
  if (path && !state.repos.includes(path)) {
    state.repos.push(path);
    renderRepos();
  }
  closePicker();
  document.querySelector("#repo-open").focus();
}

function closePicker() {
  document.querySelector("#repo-picker").hidden = true;
}

async function openPicker(path) {
  const panel = document.querySelector("#repo-picker");
  const error = document.querySelector("#picker-error");
  panel.hidden = false;
  error.hidden = true;
  let listing;
  try {
    listing = await api(`/api/dirs?path=${encodeURIComponent(path)}`);
  } catch (err) {
    error.textContent = messageOf(err);
    error.hidden = false;
    return;
  }
  state.picker = { path: listing.path, parent: listing.parent };
  document.querySelector("#picker-path").textContent = listing.path;
  document.querySelector("#picker-up").disabled = listing.parent === "";
  const here = document.querySelector("#picker-here");
  here.hidden = !listing.repo;
  const note = document.querySelector("#picker-note");
  note.hidden = !listing.truncated && listing.entries.length > 0;
  note.textContent = listing.truncated ? "Showing the first folders only." : "No folders here.";
  const list = document.querySelector("#picker-list");
  list.replaceChildren();
  for (const entry of listing.entries) {
    list.append(pickerRow(entry));
  }
}

function pickerRow(entry) {
  const item = el("li", "flex items-center justify-between gap-2 rounded-md px-2 py-1.5 hover:bg-kx-hover");
  const open = el("button", "min-w-0 flex-1 truncate text-left text-sm", entry.name);
  open.type = "button";
  open.addEventListener("click", () => openPicker(entry.path));
  item.append(open);
  if (entry.repo) {
    item.append(el("span", tagClass, "Git"));
    const blocked = entry.path.includes(",");
    const add = el("button", textButtonClass, blocked ? "Commas not supported" : "Add");
    add.type = "button";
    add.disabled = blocked;
    add.addEventListener("click", () => addRepo(entry.path));
    item.append(add);
  }
  return item;
}

function setCap(name, on) {
  const field = document.querySelector(`[data-cap="${name}"]`);
  if (field) {
    field.hidden = !on;
  }
}

function nameProblem(value) {
  if (value === "") {
    return "Enter a name.";
  }
  if (value.length > 63) {
    return "Use 63 characters or fewer.";
  }
  if (/[A-Z]/.test(value)) {
    return "Use lowercase letters only.";
  }
  if (/^-|-$/.test(value)) {
    return "Start and end with a letter or digit.";
  }
  if (!NAME_RULE.test(value)) {
    return "Use only lowercase letters, digits, and hyphens.";
  }
  return "";
}

function checkName(report) {
  const input = document.querySelector("#create-name");
  const help = document.querySelector("#name-help");
  const value = input.value.trim();
  const problem = nameProblem(value);
  const show = problem !== "" && (report || value !== "");
  if (show) {
    input.setAttribute("aria-invalid", "true");
    help.className = "text-xs text-kx-danger";
    help.textContent = problem;
  } else {
    input.removeAttribute("aria-invalid");
    help.className = "text-xs text-kx-muted";
    help.textContent = "Lowercase letters, digits, and hyphens. Up to 63 characters.";
  }
  return problem === "";
}

function createBody(form) {
  const data = new FormData(form);
  const provider = selectedProvider();
  const caps = provider ? provider.capabilities || {} : {};
  const syncOn = Boolean(caps.sync) && data.get("sync") === "on";
  const apiOn = Boolean(caps.api) && data.get("api") === "on";
  const publicField = document.querySelector('[data-cap="public"]');
  const body = {
    name: String(data.get("name") || "").trim(),
    provider: provider ? provider.name : "",
    running: Boolean(caps.running) && data.get("running") === "on",
    sync: syncOn,
    no_install: syncOn && data.get("no_install") === "on",
    dir: syncOn ? state.repos.join(",") : "",
    api: apiOn,
    public: Boolean(publicField) && !publicField.hidden && data.get("public") === "on",
  };
  if (apiOn) {
    body.api_cmd = String(data.get("api_cmd") || "").split(",").map((part) => part.trim()).filter(Boolean);
    const port = String(data.get("api_port") || "").trim();
    body.api_port = port ? Number(port) : 0;
    body.api_env = String(data.get("api_env") || "").split("\n").map((line) => line.trim()).filter(Boolean);
  }
  const template = templateValue();
  return { ...body, ...computeBody(), ...(template ? { template } : {}) };
}

function guardUnload(event) {
  event.preventDefault();
  event.returnValue = "";
}

async function onCreate(event) {
  event.preventDefault();
  const form = event.currentTarget;
  hideCreateError();
  if (!checkName(true)) {
    document.querySelector("#create-name").focus();
    return;
  }
  const invalid = checkCompute(true) || checkTemplate(true);
  if (invalid) {
    invalid.focus();
    return;
  }
  const body = createBody(form);
  state.creating = true;
  window.addEventListener("beforeunload", guardUnload);
  startProgress(body.name);
  let outcome = { error: "The connection closed before the create finished." };
  try {
    outcome = await streamCreate(body);
  } catch (err) {
    outcome = { error: messageOf(err), refused: err.refused === true };
  }
  state.creating = false;
  window.removeEventListener("beforeunload", guardUnload);
  stopTimer();
  syncCreateFields();
  if (outcome.done) {
    finishCreate(form, outcome.done);
  } else {
    failCreate(outcome);
  }
}

async function streamCreate(body) {
  let res;
  try {
    res = await fetch("/api/sandboxes", { method: "POST", headers: apiHeaders("POST"), body: JSON.stringify(body) });
  } catch {
    throw new Error("Could not reach kaxa ui. Is it still running?");
  }
  if (!res.ok) {
    const err = await readError(res);
    err.refused = true;
    throw err;
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let outcome = { error: "The connection closed before the create finished." };
  const handle = (line) => {
    if (line.trim() === "") {
      return;
    }
    const ev = JSON.parse(line);
    if (ev.step) {
      addStep(ev.step);
    } else if (ev.done) {
      outcome = { done: ev.done };
    } else if (ev.error) {
      outcome = { error: ev.error, hint: ev.hint || "", id: ev.id || "" };
    }
  };
  for (;;) {
    let chunk;
    try {
      chunk = await reader.read();
    } catch {
      break;
    }
    buffer += decoder.decode(chunk.value || new Uint8Array(), { stream: !chunk.done });
    let newline = buffer.indexOf("\n");
    while (newline >= 0) {
      handle(buffer.slice(0, newline));
      buffer = buffer.slice(newline + 1);
      newline = buffer.indexOf("\n");
    }
    if (chunk.done) {
      break;
    }
  }
  handle(buffer);
  return outcome;
}

function startProgress(name) {
  document.querySelector("#create-form").hidden = true;
  const panel = document.querySelector("#create-progress");
  panel.hidden = false;
  document.querySelector("#progress-title").textContent = `Creating ${name}`;
  document.querySelector("#progress-steps").replaceChildren();
  document.querySelector("#progress-error").hidden = true;
  const actions = document.querySelector("#progress-actions");
  actions.hidden = true;
  actions.replaceChildren();
  state.runningStep = null;
  const started = Date.now();
  const meta = document.querySelector("#progress-meta");
  meta.textContent = "Elapsed 0s";
  stopTimer();
  state.timer = setInterval(() => {
    meta.textContent = `Elapsed ${Math.round((Date.now() - started) / 1000)}s`;
  }, 1000);
  state.startedAt = started;
  document.querySelector("#progress-title").focus({ preventScroll: true });
}

function stopTimer() {
  clearInterval(state.timer);
  state.timer = null;
}

function addStep(label) {
  finishStep("done");
  const item = el("li", "flex items-center gap-2 text-sm");
  item.append(stepIcon("running"), el("span", "", label));
  document.querySelector("#progress-steps").append(item);
  state.runningStep = item;
}

function finishStep(status) {
  const item = state.runningStep;
  if (!item) {
    return;
  }
  item.firstChild.replaceWith(stepIcon(status));
  state.runningStep = null;
}

function seconds() {
  return `${Math.round((Date.now() - state.startedAt) / 1000)}s`;
}

function finishCreate(form, done) {
  finishStep("done");
  document.querySelector("#progress-meta").textContent = `Finished in ${seconds()}`;
  state.fresh = {
    provider: done.provider,
    id: done.id,
    work_dir: done.work_dir || "",
    preview_token: done.preview_token || "",
    skipped_commands: done.skipped_commands || [],
  };
  form.reset();
  state.repos = [];
  closePicker();
  renderRepos();
  checkName(false);
  document.querySelector("#create-progress").hidden = true;
  form.hidden = false;
  syncCreateFields();
  state.listLoaded = false;
  if (state.route.name === "new") {
    location.hash = sandboxHash(done.provider, done.id);
  } else {
    toast(`Created ${done.id}`);
  }
}

function failCreate(outcome) {
  if (outcome.refused) {
    document.querySelector("#create-progress").hidden = true;
    document.querySelector("#create-form").hidden = false;
    showCreateError(outcome.error);
    return;
  }
  finishStep("failed");
  document.querySelector("#progress-meta").textContent = `Failed after ${seconds()}`;
  const box = document.querySelector("#progress-error");
  box.hidden = false;
  document.querySelector("#progress-error-text").textContent = outcome.error;
  document.querySelector("#progress-error-hint").textContent = outcome.hint || "";
  const actions = document.querySelector("#progress-actions");
  actions.replaceChildren();
  const retry = el("button", primaryClass, "Edit and retry");
  retry.type = "button";
  retry.addEventListener("click", () => {
    document.querySelector("#create-progress").hidden = true;
    document.querySelector("#create-form").hidden = false;
    document.querySelector("#create-name").focus();
  });
  actions.append(retry);
  if (outcome.id) {
    const open = el("a", textButtonClass, "Open the sandbox that was kept");
    open.href = sandboxHash(selectedName(), outcome.id);
    actions.append(open);
  }
  actions.hidden = false;
}

function selectedName() {
  const provider = selectedProvider();
  return provider ? provider.name : "";
}

function showCreateError(message) {
  const box = document.querySelector("#create-error");
  box.textContent = message;
  box.hidden = false;
}

function hideCreateError() {
  const box = document.querySelector("#create-error");
  box.hidden = true;
  box.textContent = "";
}

route();

// The shell opens in its own browser tab, at #/terminal/{provider}/{name}. The tab
// connects on load and ends the shell when it is closed.
function closeTerminal() {
  const live = state.term;
  state.term = null;
  if (live) {
    if (live.socket) {
      live.socket.close();
    }
    live.term.dispose();
    live.watch.disconnect();
  }
}

window.addEventListener("pagehide", closeTerminal);

function loadScript(src) {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = src;
    script.onload = resolve;
    script.onerror = () => reject(new Error("Could not load the terminal."));
    document.head.append(script);
  });
}

function loadXterm() {
  if (!xtermReady) {
    xtermReady = loadScript("/vendor/xterm.js")
      .then(() => loadScript("/vendor/addon-fit.js"))
      .catch((err) => {
        xtermReady = null;
        throw err;
      });
  }
  return xtermReady;
}

function shellCommandText(provider, id) {
  return `kaxa --provider ${provider} ssh ${id}`;
}

// shellPanel is the Shell section of the sandbox page.
function shellPanel(detail) {
  const panel = el("section", panelClass);
  panel.setAttribute("aria-label", "Shell");
  panel.append(el("p", "text-sm font-medium", "Shell"));
  panel.append(el("p", mutedClass, "Opens a bash shell in this sandbox, in a new browser tab."));
  const link = el("a", linkButtonClass, "Open terminal");
  link.href = `#/terminal/${encodeURIComponent(detail.provider)}/${encodeURIComponent(detail.id)}`;
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  panel.append(link);
  panel.append(commandBlock(detail.shell_command || shellCommandText(detail.provider, detail.id)));
  return panel;
}

function commandBlock(command) {
  const wrap = el("div", "grid gap-2 border-t border-kx-line pt-3");
  wrap.append(el("p", mutedClass, "Prefer your own terminal? Run this command."));
  const row = el("div", "flex flex-wrap items-center gap-3");
  const code = el("code", codeBoxClass, command);
  const copy = el("button", quietClass, "Copy");
  copy.type = "button";
  copy.addEventListener("click", () => copyText(code.textContent, copy));
  row.append(code, copy);
  wrap.append(row);
  return wrap;
}

function openTerminalTab(provider, id) {
  closeTerminal();
  const root = views.terminal;
  root.replaceChildren();
  root.className = termHostClass;
  loadXterm().then(
    () => {
      if (state.route.name === "terminal" && state.route.id === id) {
        startTerminal(root, provider, id);
      }
    },
    (err) => {
      root.className = "p-4 text-sm text-kx-danger";
      root.textContent = messageOf(err);
    },
  );
}

function startTerminal(root, provider, id) {
  const term = new Terminal({ cursorBlink: true, fontFamily: "IBM Plex Mono, ui-monospace, monospace", fontSize: 14, theme: termTheme });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(root);
  fit.fit();
  const watch = new ResizeObserver(() => fit.fit());
  watch.observe(root);
  const live = { term, watch, socket: null, waiting: false };
  state.term = live;
  const encoder = new TextEncoder();
  term.onData((data) => {
    if (live.socket && live.socket.readyState === WebSocket.OPEN) {
      live.socket.send(encoder.encode(data));
    } else if (live.waiting && data === "\r") {
      connectTerminal(live, provider, id);
    }
  });
  term.onResize(({ rows, cols }) => {
    if (live.socket && live.socket.readyState === WebSocket.OPEN) {
      live.socket.send(JSON.stringify({ resize: { rows, cols } }));
    }
  });
  connectTerminal(live, provider, id);
}

// notifyTerminal writes a kaxa line in the terminal and waits for Enter.
function notifyTerminal(live, text, failed) {
  live.waiting = true;
  live.term.write(`\r\n\x1b[${failed ? 31 : 90}m[kaxa] ${text} Press Enter to open a new shell.\x1b[0m\r\n`);
}

async function connectTerminal(live, provider, id) {
  live.waiting = false;
  live.term.reset();
  try {
    const grant = await api(sandboxPath({ provider, id }, "terminal"), { method: "POST", body: {} });
    if (state.term === live) {
      attachTerminal(live, grant);
    }
  } catch (err) {
    if (state.term === live) {
      notifyTerminal(live, messageOf(err), true);
    }
  }
}

function attachTerminal(live, grant) {
  const { term } = live;
  const query = `ticket=${encodeURIComponent(grant.ticket)}&rows=${term.rows}&cols=${term.cols}`;
  const socket = new WebSocket(`ws://${location.host}/ws/terminal?${query}`);
  socket.binaryType = "arraybuffer";
  live.socket = socket;
  let opened = false;
  socket.addEventListener("open", () => {
    opened = true;
    term.focus();
  });
  socket.addEventListener("message", (event) => term.write(new Uint8Array(event.data)));
  socket.addEventListener("close", () => {
    if (state.term !== live || live.socket !== socket) {
      return;
    }
    live.socket = null;
    notifyTerminal(live, opened ? "Shell closed." : "Could not open the terminal.", !opened);
  });
}
