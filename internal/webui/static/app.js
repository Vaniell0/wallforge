"use strict";

// Items are a union of Steam Workshop subscriptions and local library
// entries. Steam items carry {id, type, title, tags, has_preview,
// broken}; library items carry {id, kind, title, root}. Both sources
// pass through /api/apply → apply.ByInput and /preview/{id}.
const state = {
  items: [],
  lastApplied: "",
  source: "all", // all | workshop | library
};

const grid = document.getElementById("grid");
const empty = document.getElementById("empty");
const filter = document.getElementById("filter");
const typeFilter = document.getElementById("type-filter");
const sourceFilter = document.getElementById("source-filter");
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
  setStatus("Loading…");
  try {
    // Parallel fetches; each can fail independently.
    const [steamResP, libraryResP, statusResP] = [
      fetchJSON("/api/items").catch(() => []),
      fetchJSON("/api/library").catch(() => []),
      fetchJSON("/api/status").catch(() => ({})),
    ];
    const [steamItems, libraryItems, status] = await Promise.all([
      steamResP, libraryResP, statusResP,
    ]);

    // Normalise both into a common shape used by the card renderer.
    const steam = (steamItems || []).map((it) => ({
      source: "workshop",
      id: it.id,
      title: it.title,
      kind: it.type,
      tags: it.tags || [],
      hasPreview: !!it.has_preview,
      broken: !!it.broken,
    }));
    const library = (libraryItems || []).map((it) => ({
      source: "library",
      id: it.id,
      title: it.title,
      kind: it.kind,
      tags: [],
      hasPreview: true, // local file; /preview serves it directly
      broken: false,
      root: it.root,
    }));

    state.items = [...steam, ...library];
    state.lastApplied = status.last_applied || "";
    render();
    setStatus(
      `${steam.length} workshop + ${library.length} library.`,
      "ok",
    );
  } catch (err) {
    setStatus(err.message, "err");
    grid.innerHTML = "";
    empty.hidden = false;
  }
}

function render() {
  const q = filter.value.trim().toLowerCase();
  const t = typeFilter.value;
  const src = sourceFilter.value;
  const filtered = state.items.filter((it) => {
    if (src && src !== "all" && it.source !== src) return false;
    if (t && it.kind !== t) return false;
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
  if (it.hasPreview) {
    preview.style.backgroundImage = `url('/preview/${encodeURIComponent(it.id)}')`;
  }

  const body = document.createElement("div");
  body.className = "body";
  const h2 = document.createElement("h2");
  h2.textContent = it.title || "(untitled)";
  const meta = document.createElement("div");
  meta.className = "meta";
  if (it.kind) {
    const badge = document.createElement("span");
    badge.className = "badge";
    badge.textContent = it.kind;
    meta.appendChild(badge);
  }
  const sourceBadge = document.createElement("span");
  sourceBadge.className = "badge source-" + it.source;
  sourceBadge.textContent = it.source;
  meta.appendChild(sourceBadge);

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
sourceFilter.addEventListener("change", render);
stopBtn.addEventListener("click", stopAll);
reloadBtn.addEventListener("click", loadItems);

loadItems();
