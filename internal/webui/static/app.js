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
const reloadBtn = document.getElementById("reload-btn");
const statusEl = document.getElementById("status");
const powerEl = document.getElementById("power");
const powerAcEl = document.getElementById("power-ac");
const powerProfileEl = document.getElementById("power-profile");
const powerStateEl = document.getElementById("power-state");
const powerPauseBtn = document.getElementById("power-pause");
const powerResumeBtn = document.getElementById("power-resume");

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
    // Probe the preview URL; only paint the background once we know
    // the server really returned an image. A missing or corrupted
    // preview falls back to the "no preview" placeholder styled via
    // CSS — no more empty black cards.
    const src = `/preview/${encodeURIComponent(it.id)}`;
    const probe = new Image();
    probe.onload = () => {
      preview.style.backgroundImage = `url('${src}')`;
    };
    probe.onerror = () => {
      preview.classList.add("no-preview");
    };
    probe.src = src;
  } else {
    preview.classList.add("no-preview");
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

filter.addEventListener("input", render);
typeFilter.addEventListener("change", render);
sourceFilter.addEventListener("change", render);
reloadBtn.addEventListener("click", () => {
  loadItems();
  loadPower();
});
powerPauseBtn.addEventListener("click", () => powerAction("pause"));
powerResumeBtn.addEventListener("click", () => powerAction("resume"));

// Power state isn't load-bearing for the wallpaper grid, so we render
// it independently and don't block the rest of the UI on it.
async function loadPower() {
  try {
    const p = await fetchJSON("/api/power");
    renderPower(p);
  } catch (err) {
    // Hide silently — endpoint absence shouldn't disrupt the main UI
    // (e.g. older binaries served from a stale package).
    powerEl.hidden = true;
  }
}

function renderPower(p) {
  powerEl.hidden = false;
  powerAcEl.textContent = p.ac ? "ac" : "battery";
  powerAcEl.className = "badge power-" + (p.ac ? "ac" : "battery");
  powerProfileEl.textContent = p.profile;
  powerProfileEl.className = "badge power-profile-" + p.profile;

  // State badge: manual pause wins over auto state. Auto modes are
  // "low-power" (reduced opts/fps) or "paused" (full stop). Normal
  // mode hides the badge entirely.
  let stateText = "";
  let stateClass = "badge";
  if (p.user_paused) {
    stateText = "paused (manual)";
    stateClass = "badge power-state-paused";
  } else if (p.mode === "paused") {
    stateText = "paused: " + p.reason;
    stateClass = "badge power-state-auto";
  } else if (p.mode === "low-power") {
    stateText = "low-power: " + p.reason;
    stateClass = "badge power-state-low";
  }
  if (stateText) {
    powerStateEl.textContent = stateText;
    powerStateEl.className = stateClass;
    powerStateEl.hidden = false;
  } else {
    powerStateEl.hidden = true;
  }

  const isPaused = p.user_paused || p.mode === "paused";
  powerPauseBtn.disabled = isPaused;
  powerResumeBtn.disabled = !isPaused && !p.last_applied;
}

async function powerAction(kind) {
  setStatus(kind === "pause" ? "Pausing…" : "Resuming…");
  try {
    await fetchJSON("/api/power/" + kind, { method: "POST" });
    setStatus(kind === "pause" ? "Paused." : "Resumed.", "ok");
    await loadPower();
  } catch (err) {
    setStatus(err.message, "err");
  }
}

loadItems();
loadPower();
// Lightweight refresh: re-poll power state every 30s so a profile
// switch outside the UI (e.g. via powerprofilesctl) eventually surfaces
// without forcing a manual reload.
setInterval(loadPower, 30_000);
