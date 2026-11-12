// Navigasi panel admin: sidebar, ganti panel, hash routing, drawer mobile, dan pil status helper.
// Dipisah dari admin.js supaya logika data tidak perlu diubah saat tata letak berganti.

const NAV_PANELS = {
  dashboard: ["Dashboard", "Angka penting, hal yang butuh tindakan, dan fakta helper, aplikasi, database, Cloudflare."],
  server: ["Server", "Aksi sistem lewat helper root: service, backup, log, doctor, konektor cloudflared."],
  cloudflare: ["Akun Cloudflare", "Profil akun Cloudflare beserta API token dan status Worker."],
  domains: ["Domain temp mail", "Domain beserta status routing catch-all ke Worker."],
  tunnel: ["Tunnel", "Tunnel Cloudflare dan web host yang dipakai Worker."],
  copies: ["Salinan email", "Tembusan email opsional ke alamat lain."],
  import: ["Impor state", "Muat profil dan domain dari cloudflare-state.json lama."],
  users: ["Pengguna", "Akun panel beserta role, ganti role, reset password, dan hapus akun."],
  naming: ["Aturan nama", "Pola local-part inbox yang dibuat otomatis saat username tidak diisi."],
  diagnostics: ["Diagnostik", "Fakta helper, aplikasi, database, dan Cloudflare dalam satu layar baca."],
};
const NAV_DEFAULT = "dashboard";

const navEl = document.getElementById("shellNav");
const navScrim = document.getElementById("navScrim");
const navToggle = document.getElementById("navToggle");
const navUser = document.getElementById("navUser");
const viewTitle = document.getElementById("viewTitle");
const viewSub = document.getElementById("viewSub");
const helperPill = document.getElementById("helperPill");
const gateEl = document.getElementById("gate");
const panelEl = document.getElementById("panel");
const navButtons = [...document.querySelectorAll("#shellNav [data-panel]")];
const navPanels = [...document.querySelectorAll("#panel .panel[data-panel]")];

function panelFromHash() {
  const name = (window.location.hash || "").replace(/^#/, "");
  return Object.prototype.hasOwnProperty.call(NAV_PANELS, name) ? name : NAV_DEFAULT;
}

function showPanel(name, options) {
  const domPanels = [...document.querySelectorAll("#panel .panel[data-panel]")];
  const domNames = domPanels.map((panel) => panel.dataset.panel);
  let target = Object.prototype.hasOwnProperty.call(NAV_PANELS, name) ? name : NAV_DEFAULT;
  if (!domNames.includes(target)) {
    // Markup yang dimuat browser lebih tua dari skrip ini: jangan biarkan layar kosong.
    const fallback = domNames.includes(NAV_DEFAULT) ? NAV_DEFAULT : domNames[0];
    if (fallback) {
      stalePanelNotice(target);
      target = fallback;
    }
  }
  domPanels.forEach((panel) => {
    panel.hidden = panel.dataset.panel !== target;
  });
  navButtons.forEach((button) => {
    if (button.dataset.panel === target) {
      button.setAttribute("aria-current", "page");
    } else {
      button.removeAttribute("aria-current");
    }
  });
  const meta = NAV_PANELS[target] || [target, ""];
  viewTitle.textContent = meta[0];
  viewSub.textContent = meta[1];
  document.title = meta[0] + " - Panel TempMail By JhopanStore";
  if (!options || !options.silent) {
    const heading = document.querySelector('#panel .panel[data-panel="' + target + '"] h2');
    if (heading) {
      heading.setAttribute("tabindex", "-1");
      if (options && options.focus) heading.focus({ preventScroll: true });
    }
  }
  closeDrawer();
}

// Hanya muncul kalau panel yang diminta tidak ada di markup (halaman dari cache lama).
function stalePanelNotice(requested) {
  if (document.getElementById("staleNotice")) return;
  const box = document.createElement("div");
  box.id = "staleNotice";
  box.className = "state error";
  box.setAttribute("role", "alert");
  const text = document.createElement("div");
  text.className = "state-text";
  text.textContent = 'Panel "' + requested + '" belum ada di halaman ini, jadi halaman yang terbuka masih versi lama dari cache browser.';
  const row = document.createElement("div");
  row.className = "row-actions";
  row.style.marginTop = "8px";
  const reload = document.createElement("button");
  reload.type = "button";
  reload.className = "btn btn-p small";
  reload.textContent = "Muat ulang halaman";
  reload.addEventListener("click", () => window.location.reload());
  row.append(reload);
  box.append(text, row);
  document.getElementById("panel").prepend(box);
  if (!sessionStorage.getItem("adminStaleReload")) {
    sessionStorage.setItem("adminStaleReload", "1");
    setTimeout(() => window.location.reload(), 1200);
  }
}

function openDrawer() {
  document.body.classList.add("nav-open");
  navToggle.setAttribute("aria-expanded", "true");
  navScrim.hidden = false;
}

function closeDrawer() {
  if (!document.body.classList.contains("nav-open")) return;
  document.body.classList.remove("nav-open");
  navToggle.setAttribute("aria-expanded", "false");
  navScrim.hidden = true;
}

function applyGateState() {
  const gated = gateEl ? !gateEl.hidden : false;
  document.body.classList.toggle("gated", gated);
  if (gated) closeDrawer();
}

function renderNavUser() {
  let email = "-";
  try {
    email = (JSON.parse(localStorage.getItem("auth_user") || "{}").email) || "unknown";
  } catch (error) {
    email = "unknown";
  }
  navUser.textContent = "Masuk sebagai " + email;
}

function setHelperPill(installed, pending) {
  if (!helperPill) return;
  helperPill.classList.remove("on", "off");
  if (installed === true) {
    helperPill.textContent = "helper: terpasang" + (pending ? " (" + pending + " request)" : "");
    helperPill.classList.add("on");
  } else if (installed === false) {
    helperPill.textContent = "helper: belum dipasang";
    helperPill.classList.add("off");
  } else {
    helperPill.textContent = "helper: ?";
  }
}

async function refreshHelperPill() {
  if (!helperPill || document.body.classList.contains("gated")) return;
  try {
    const response = await fetch("/api/admin/system/status", {
      headers: { Authorization: "Bearer " + (localStorage.getItem("auth_token") || "") },
    });
    if (!response.ok) {
      setHelperPill(null);
      return;
    }
    const body = await response.json();
    setHelperPill(Boolean(body.helper && body.helper.installed), (body.helper && body.helper.pending) || 0);
  } catch (error) {
    setHelperPill(null);
  }
}

function wireOutputCopy() {
  const button = document.getElementById("svOutputCopy");
  const output = document.getElementById("svOutput");
  const head = document.getElementById("svOutputHead");
  if (!button || !output || !head) return;
  const syncHead = () => {
    head.hidden = output.hidden || !output.textContent.trim();
  };
  button.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(output.textContent);
      button.textContent = "Tersalin";
    } catch (error) {
      button.textContent = "Gagal salin";
    }
    setTimeout(() => { button.textContent = "Salin keluaran"; }, 1500);
  });
  new MutationObserver(syncHead).observe(output, { attributes: true, childList: true, characterData: true, subtree: true });
  syncHead();
}

function initNav() {
  renderNavUser();
  wireOutputCopy();

  navButtons.forEach((button) => {
    button.addEventListener("click", () => {
      const name = button.dataset.panel;
      if (window.location.hash.replace(/^#/, "") === name) {
        showPanel(name, { silent: true });
      } else {
        window.location.hash = name;
      }
    });
  });

  window.addEventListener("hashchange", () => showPanel(panelFromHash(), { silent: true }));

  navToggle.addEventListener("click", () => {
    if (document.body.classList.contains("nav-open")) closeDrawer();
    else openDrawer();
  });
  navScrim.addEventListener("click", closeDrawer);
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && document.body.classList.contains("nav-open")) closeDrawer();
  });
  window.addEventListener("resize", () => {
    if (window.innerWidth > 900) closeDrawer();
  });

  if (gateEl) {
    new MutationObserver(applyGateState).observe(gateEl, { attributes: true, attributeFilter: ["hidden"] });
  }
  applyGateState();

  const reload = document.getElementById("svReloadBtn");
  if (reload) reload.addEventListener("click", () => setTimeout(refreshHelperPill, 400));

  showPanel(panelFromHash(), { silent: true });
  refreshHelperPill();
  sessionStorage.removeItem("adminStaleReload");
  panelEl.dataset.navReady = "1";
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initNav);
} else {
  initNav();
}
