"use strict";

const state = {
  items: [],
  lastApplied: "",
};

const grid = document.getElementById("grid");
const empty = document.getElementById("empty");
const filter = document.getElementById("filter");
const typeFilter = document.getElementById("type-filter");
const stopBtn = document.getElementById("stop-btn");
const reloadBtn = document.getElementById("reload-btn");
const statusEl = document.getElementById("status");

function setStatus(msg, kind = "") {
  statusEl.textContent = msg;
  statusEl.className = "status" + (kind ? " " + kind : "");
}

async function fetchJSON(url, opts) {
  const res = await fetch(url, opts);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body.trim() || res.statusText}`);
  }
  const ct = res.headers.get("content-type") || "";
  return ct.includes("json") ? res.json() : res.text();
}

async function loadItems() {
  setStatus("Loading subscriptions…");
  try {
    state.items = await fetchJSON("/api/items");
    const status = await fetchJSON("/api/status");
    state.lastApplied = status.last_applied || "";
    render();
    setStatus(`${state.items.length} subscription(s).`, "ok");
  } catch (err) {
    setStatus(err.message, "err");
    grid.innerHTML = "";
    empty.hidden = false;
  }
}

function render() {
  const q = filter.value.trim().toLowerCase();
  const t = typeFilter.value;
  const filtered = state.items.filter((it) => {
    if (t && it.type !== t) return false;
    if (!q) return true;
    if ((it.title || "").toLowerCase().includes(q)) return true;
    if ((it.tags || []).some((tag) => tag.toLowerCase().includes(q))) return true;
    if (it.id.includes(q)) return true;
    return false;
  });

  grid.innerHTML = "";
  empty.hidden = filtered.length > 0;
  if (filtered.length === 0) return;

  const frag = document.createDocumentFragment();
  for (const it of filtered) {
    frag.appendChild(renderCard(it));
  }
  grid.appendChild(frag);
}

function renderCard(it) {
  const card = document.createElement("article");
  card.className = "card";
  if (it.broken) card.classList.add("broken");
  if (it.id === state.lastApplied) card.classList.add("active");
  card.dataset.id = it.id;

  const preview = document.createElement("div");
  preview.className = "preview";
  if (it.has_preview) {
    preview.style.backgroundImage = `url('/preview/${encodeURIComponent(it.id)}')`;
  }

  const body = document.createElement("div");
  body.className = "body";
  const h2 = document.createElement("h2");
  h2.textContent = it.title || "(untitled)";
  const meta = document.createElement("div");
  meta.className = "meta";
  if (it.type) {
    const badge = document.createElement("span");
    badge.className = "badge";
    badge.textContent = it.type;
    meta.appendChild(badge);
  }
  const idSpan = document.createElement("span");
  idSpan.textContent = it.id;
  meta.appendChild(idSpan);

  body.append(h2, meta);
  card.append(preview, body);

  if (!it.broken) {
    card.addEventListener("click", () => applyItem(it.id));
  }
  return card;
}

async function applyItem(id) {
  setStatus(`Applying ${id}…`);
  try {
    const res = await fetchJSON("/api/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ input: id }),
    });
    state.lastApplied = id;
    document.querySelectorAll(".card.active").forEach((c) => c.classList.remove("active"));
    const card = document.querySelector(`.card[data-id="${CSS.escape(id)}"]`);
    if (card) card.classList.add("active");
    const title = res.title || id;
    setStatus(`Applied ${title} → ${res.backend}.`, "ok");
  } catch (err) {
    setStatus(err.message, "err");
  }
}

async function stopAll() {
  setStatus("Stopping backends…");
  try {
    const res = await fetchJSON("/api/stop", { method: "POST" });
    state.lastApplied = "";
    document.querySelectorAll(".card.active").forEach((c) => c.classList.remove("active"));
    if (res.ok) {
      setStatus("All backends stopped.", "ok");
    } else {
      setStatus("Stopped with warnings: " + (res.errors || []).join("; "), "err");
    }
  } catch (err) {
    setStatus(err.message, "err");
  }
}

filter.addEventListener("input", render);
typeFilter.addEventListener("change", render);
stopBtn.addEventListener("click", stopAll);
reloadBtn.addEventListener("click", loadItems);

loadItems();
