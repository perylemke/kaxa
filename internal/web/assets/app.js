"use strict";

const session = meta("kaxa-session");
const csrf = meta("kaxa-csrf");
const views = {
  list: document.querySelector("#view-list"),
  detail: document.querySelector("#view-detail"),
  create: document.querySelector("#view-create"),
};
const state = { providers: [], sandboxes: [], warnings: [], detail: null, destroyArmed: false };

const navOn = "rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm text-kx-on-button";
const navOff = "rounded-lg border border-kx-line bg-kx-surface-2 px-3.5 py-2 text-sm text-kx-text hover:bg-kx-hover";
const noticeClass = "break-words rounded-lg border border-kx-button bg-kx-button/15 px-3.5 py-3 text-sm";
const quietClass = "rounded-lg border border-kx-line bg-kx-surface-2 px-3.5 py-2 text-sm text-kx-text hover:bg-kx-hover";
const dangerClass = "rounded-lg border border-transparent bg-kx-danger-bg px-3.5 py-2 text-sm text-kx-danger";
const primaryClass = "rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm text-kx-on-button disabled:cursor-not-allowed disabled:opacity-45";
const rowClass = "flex w-full items-center gap-4 border-0 bg-transparent px-1 py-3 text-left hover:bg-kx-hover";
const livePillClass = "rounded-full bg-kx-accent/15 px-2.5 py-0.5 text-xs font-medium text-kx-accent";
const idlePillClass = "rounded-full bg-kx-surface-2 px-2.5 py-0.5 text-xs font-medium text-kx-muted";
const linkClass = "break-all text-sm text-kx-accent underline decoration-kx-accent/40 underline-offset-2";
const monoClass = "break-all font-mono text-sm";

document.querySelector("#nav-list").addEventListener("click", () => show("list"));
document.querySelector("#nav-create").addEventListener("click", () => show("create"));
document.querySelector("#create-form").addEventListener("submit", onCreate);
document.querySelector("#create-provider").addEventListener("change", syncCreateFields);
document.querySelector("#create-sync").addEventListener("change", syncCreateFields);
document.querySelector("#create-api").addEventListener("change", syncCreateFields);

function meta(name) {
  const el = document.querySelector(`meta[name="${name}"]`);
  return el ? el.content : "";
}

function show(name) {
  for (const [key, el] of Object.entries(views)) {
    el.hidden = key !== name;
  }
  const onList = name === "list" || name === "detail";
  document.querySelector("#nav-list").className = onList ? navOn : navOff;
  document.querySelector("#nav-create").className = name === "create" ? navOn : navOff;
  if (name === "list") {
    refresh().catch(showError);
  }
  if (name === "create") {
    fillProviders();
  }
}

async function api(path, options = {}) {
  const method = options.method || "GET";
  const headers = { "X-Kaxa-Session": session };
  const init = { method, headers };
  if (method !== "GET") {
    headers["X-Kaxa-CSRF"] = csrf;
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body || {});
  }
  const res = await fetch(path, init);
  const text = await res.text();
  let body = {};
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = {};
    }
  }
  if (!res.ok) {
    const err = new Error(body.error || "request failed");
    err.hint = body.hint || "";
    throw err;
  }
  return body;
}

function showError(err) {
  const notice = document.querySelector("#notice");
  notice.hidden = false;
  notice.className = noticeClass;
  notice.textContent = err && err.hint ? `${err.message} ${err.hint}` : (err && err.message) || "request failed";
}

function clearError() {
  const notice = document.querySelector("#notice");
  notice.hidden = true;
  notice.textContent = "";
}

async function refresh() {
  clearError();
  const providers = await api("/api/providers");
  let listed = { sandboxes: [], warnings: [] };
  try {
    listed = await api("/api/sandboxes");
  } catch (err) {
    showError(err);
  }
  state.providers = providers.providers || [];
  state.sandboxes = listed.sandboxes || [];
  state.warnings = listed.warnings || [];
  renderWarnings();
  renderList();
  fillProviders();
}

function renderWarnings() {
  const root = document.querySelector("#warnings");
  root.replaceChildren();
  for (const warning of state.warnings) {
    const p = document.createElement("p");
    p.className = noticeClass;
    p.textContent = warning;
    root.append(p);
  }
}

function renderList() {
  const root = views.list;
  root.replaceChildren();
  root.className = "grid gap-4";
  const head = document.createElement("div");
  head.className = "flex items-baseline justify-between gap-4";
  const title = document.createElement("h1");
  title.className = "text-xl tracking-tight";
  title.textContent = "Sandboxes";
  head.append(title);
  const count = state.sandboxes.length;
  if (count > 0) {
    const qty = document.createElement("p");
    qty.className = "text-sm text-kx-muted";
    qty.textContent = count === 1 ? "1 sandbox" : `${count} sandboxes`;
    head.append(qty);
  }
  root.append(head);
  if (count === 0) {
    const empty = document.createElement("p");
    empty.className = "text-sm text-kx-muted";
    empty.textContent = "No sandboxes yet.";
    root.append(empty, actionButton("Create sandbox", () => show("create"), "justify-self-start rounded-lg border border-transparent bg-kx-button px-3.5 py-2 text-sm text-kx-on-button"));
    return;
  }
  const list = document.createElement("div");
  list.className = "divide-y divide-kx-line";
  for (const box of state.sandboxes) {
    list.append(sandboxRow(box));
  }
  root.append(list);
}

function sandboxRow(box) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = rowClass;
  const name = document.createElement("span");
  name.className = "min-w-0 flex-1 font-medium";
  name.textContent = box.id || "";
  const provider = document.createElement("span");
  provider.className = "text-sm text-kx-muted";
  provider.textContent = box.provider || "";
  button.append(name, provider, statusPill(box.status));
  button.addEventListener("click", () => {
    openDetail(box.provider, box.id).catch(showError);
  });
  return button;
}

function statusPill(status) {
  const span = document.createElement("span");
  const live = status === "running" || status === "warm";
  span.className = live ? livePillClass : idlePillClass;
  span.textContent = status || "";
  return span;
}

async function openDetail(provider, id) {
  clearError();
  state.destroyArmed = false;
  state.detail = await api(`/api/sandboxes/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`);
  renderDetail();
  show("detail");
}

function renderDetail() {
  const detail = state.detail;
  const root = views.detail;
  root.replaceChildren();
  root.className = "grid gap-4";
  root.append(actionButton("Back to sandboxes", () => show("list"), "bg-transparent px-0 py-0 text-sm text-kx-accent"));
  const titleRow = document.createElement("div");
  titleRow.className = "flex flex-wrap items-center gap-3";
  const title = document.createElement("h1");
  title.className = "text-xl tracking-tight";
  title.textContent = detail.id || "";
  titleRow.append(title, statusPill(detail.status));
  root.append(titleRow);
  const provider = document.createElement("p");
  provider.className = "text-sm text-kx-muted";
  provider.textContent = detail.provider || "";
  root.append(provider);
  if (detail.url) {
    const link = document.createElement("a");
    link.href = detail.url;
    link.textContent = detail.url;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.className = linkClass;
    root.append(link);
  }
  const caps = detail.capabilities || {};
  const dl = document.createElement("dl");
  dl.className = "grid grid-cols-[minmax(7.5rem,11rem)_1fr] gap-x-4 gap-y-2";
  if (detail.auth) {
    appendPair(dl, "URL auth", detail.auth);
  }
  for (const field of detail.fields || []) {
    appendPair(dl, field.label, field.text || field.at || "");
  }
  if (detail.created_at) {
    appendPair(dl, "Created", detail.created_at);
  }
  if (detail.updated_at) {
    appendPair(dl, "Updated", detail.updated_at);
  }
  if (dl.childElementCount > 0) {
    root.append(dl);
  }
  const actions = document.createElement("div");
  actions.className = "flex flex-wrap gap-2";
  if (caps.restart) {
    actions.append(actionButton("Restart", () => runAction("restart"), quietClass));
  }
  if (caps.upgrade) {
    actions.append(actionButton("Upgrade", () => runAction("upgrade"), quietClass));
  }
  if (caps.public_url) {
    actions.append(actionButton("Public URL", () => setAuth("public"), quietClass));
    actions.append(actionButton("Account URL", () => setAuth("sprite"), quietClass));
  }
  if (actions.childElementCount > 0) {
    root.append(actions);
  }
  root.append(destroyZone(detail));
}

function destroyZone(detail) {
  const zone = document.createElement("div");
  zone.className = "mt-6 grid gap-3 border-t border-kx-line pt-6";
  if (!state.destroyArmed) {
    zone.append(actionButton("Destroy", armDestroy, dangerClass));
    return zone;
  }
  const copy = document.createElement("p");
  copy.className = "text-sm";
  copy.textContent = `Destroy ${detail.provider}/${detail.id}? This cannot be undone.`;
  const row = document.createElement("div");
  row.className = "flex flex-wrap gap-2";
  row.append(
    actionButton("Cancel", cancelDestroy, quietClass),
    actionButton(`Destroy ${detail.id}`, onDestroy, dangerClass),
  );
  zone.append(copy, row);
  return zone;
}

function armDestroy() {
  state.destroyArmed = true;
  renderDetail();
}

function cancelDestroy() {
  state.destroyArmed = false;
  renderDetail();
}

function appendPair(dl, label, value) {
  const dt = document.createElement("dt");
  dt.className = "text-sm text-kx-muted";
  dt.textContent = label;
  const dd = document.createElement("dd");
  dd.className = "text-sm";
  dd.textContent = value;
  dl.append(dt, dd);
}

function actionButton(label, onClick, className) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.className = className;
  button.addEventListener("click", () => {
    Promise.resolve(onClick()).catch(showError);
  });
  return button;
}

async function runAction(action) {
  const detail = state.detail;
  await api(sandboxPath(detail, action), { method: "POST", body: {} });
  await openDetail(detail.provider, detail.id);
}

async function setAuth(auth) {
  const detail = state.detail;
  state.destroyArmed = false;
  state.detail = await api(sandboxPath(detail, "url-auth"), { method: "POST", body: { auth } });
  renderDetail();
  show("detail");
}

async function onDestroy() {
  const detail = state.detail;
  if (!detail) {
    return;
  }
  await api(sandboxPath(detail, "destroy"), { method: "POST", body: { confirm: detail.id } });
  state.detail = null;
  state.destroyArmed = false;
  show("list");
}

function sandboxPath(detail, action) {
  const base = `/api/sandboxes/${encodeURIComponent(detail.provider)}/${encodeURIComponent(detail.id)}`;
  return action ? `${base}/${action}` : base;
}

function fillProviders() {
  const select = document.querySelector("#create-provider");
  const current = select.value;
  select.replaceChildren();
  for (const provider of state.providers) {
    const option = document.createElement("option");
    option.value = provider.name;
    option.textContent = provider.available ? provider.name : `${provider.name} (unavailable)`;
    option.disabled = !provider.available;
    if (provider.default && provider.available) {
      option.selected = true;
    }
    select.append(option);
  }
  const stillThere = [...select.options].some((option) => option.value === current && !option.disabled);
  if (current && stillThere) {
    select.value = current;
  }
  syncCreateFields();
}

function selectedProvider() {
  const name = document.querySelector("#create-provider").value;
  return state.providers.find((provider) => provider.name === name) || null;
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
  document.querySelector("#create-submit").disabled = !provider || !provider.available;
}

function setCap(name, on) {
  const field = document.querySelector(`[data-cap="${name}"]`);
  if (field) {
    field.hidden = !on;
  }
}

async function onCreate(event) {
  event.preventDefault();
  clearError();
  const data = new FormData(event.currentTarget);
  const provider = selectedProvider();
  const caps = provider ? provider.capabilities || {} : {};
  const syncOn = Boolean(caps.sync) && data.get("sync") === "on";
  const apiOn = Boolean(caps.api) && data.get("api") === "on";
  const body = {
    name: String(data.get("name") || "").trim(),
    provider: provider ? provider.name : "",
    running: Boolean(caps.running) && data.get("running") === "on",
    sync: syncOn,
    no_install: syncOn && data.get("no_install") === "on",
    dir: syncOn ? String(data.get("dir") || "").trim() : "",
    api: apiOn,
    public: publicChecked(data),
  };
  if (apiOn) {
    body.api_cmd = String(data.get("api_cmd") || "").split(",").map((part) => part.trim()).filter(Boolean);
    const port = String(data.get("api_port") || "").trim();
    body.api_port = port ? Number(port) : 0;
    body.api_env = String(data.get("api_env") || "").split("\n").map((line) => line.trim()).filter(Boolean);
  }
  const button = document.querySelector("#create-submit");
  button.disabled = true;
  const previous = button.textContent;
  button.textContent = "Creating…";
  try {
    renderCreated(await api("/api/sandboxes", { method: "POST", body }));
    event.currentTarget.reset();
  } catch (err) {
    showError(err);
  } finally {
    button.textContent = previous;
    syncCreateFields();
  }
}

function publicChecked(data) {
  const field = document.querySelector('[data-cap="public"]');
  return field && !field.hidden && data.get("public") === "on";
}

function renderCreated(created) {
  const root = document.querySelector("#create-result");
  root.hidden = false;
  root.replaceChildren();
  const title = document.createElement("h2");
  title.className = "text-lg tracking-tight";
  title.textContent = `Created ${created.provider}/${created.id}`;
  root.append(title);
  if (created.url) {
    const link = document.createElement("a");
    link.href = created.url;
    link.textContent = created.url;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.className = linkClass;
    root.append(link);
  }
  if (created.work_dir) {
    const dir = document.createElement("p");
    dir.className = monoClass;
    dir.textContent = created.work_dir;
    root.append(dir);
  }
  if (created.preview_token) {
    const token = document.createElement("p");
    token.className = monoClass;
    token.textContent = `Preview token (shown once): ${created.preview_token}`;
    root.append(token);
  }
  for (const command of created.skipped_commands || []) {
    const skipped = document.createElement("p");
    skipped.className = "text-sm text-kx-muted";
    skipped.textContent = `Skipped: ${command.join(" ")}`;
    root.append(skipped);
  }
  root.append(actionButton("Open sandbox", () => openDetail(created.provider, created.id), primaryClass));
}

show("list");
