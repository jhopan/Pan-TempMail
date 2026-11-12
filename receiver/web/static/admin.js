// Admin Cloudflare panel. Auth, token storage and the 401 redirect come from API in /app/app.js.
const CF_BASE = '/api/admin/cloudflare';
const SYS_BASE = '/api/admin/system';
const REQUEST_TIMEOUT_MS = 30000;
const HEALTH_TIMEOUT_MS = 30000;
const HEALTH_INTERVAL_MS = 1500;

const state = {
  profiles: [],
  domains: [],
  users: [],
  tunnel: null,
  copies: [],
  workerName: '',
  workerCompatDate: '',
  system: null,
  systemHelper: null,
};

const ui = {
  gate: document.getElementById('gate'),
  panel: document.getElementById('panel'),
  logoutBtn: document.getElementById('logoutBtn'),
  modal: document.getElementById('modal'),
  modalTitle: document.getElementById('modalTitle'),
  modalBody: document.getElementById('modalBody'),
  modalClose: document.getElementById('modalClose'),
  testConnBtn: document.getElementById('testConnBtn'),
  statusOut: document.getElementById('statusOut'),
  pfReloadBtn: document.getElementById('pfReloadBtn'),
  profilesState: document.getElementById('profilesState'),
  profilesWrap: document.getElementById('profilesWrap'),
  profilesBody: document.getElementById('profilesBody'),
  profilesMsg: document.getElementById('profilesMsg'),
  profileForm: document.getElementById('profileForm'),
  pfId: document.getElementById('pfId'),
  pfLabel: document.getElementById('pfLabel'),
  pfAccount: document.getElementById('pfAccount'),
  pfToken: document.getElementById('pfToken'),
  pfActivate: document.getElementById('pfActivate'),
  pfAddBtn: document.getElementById('pfAddBtn'),
  dmReloadBtn: document.getElementById('dmReloadBtn'),
  domainsState: document.getElementById('domainsState'),
  domainsWrap: document.getElementById('domainsWrap'),
  domainsBody: document.getElementById('domainsBody'),
  domainsMsg: document.getElementById('domainsMsg'),
  domainForm: document.getElementById('domainForm'),
  dmDomain: document.getElementById('dmDomain'),
  dmProfile: document.getElementById('dmProfile'),
  dmAddBtn: document.getElementById('dmAddBtn'),
  tnReloadBtn: document.getElementById('tnReloadBtn'),
  tunnelState: document.getElementById('tunnelState'),
  tunnelWrap: document.getElementById('tunnelWrap'),
  tunnelInfo: document.getElementById('tunnelInfo'),
  tunnelMsg: document.getElementById('tunnelMsg'),
  tunnelForm: document.getElementById('tunnelForm'),
  tnWebHost: document.getElementById('tnWebHost'),
  tnName: document.getElementById('tnName'),
  tnSaveBtn: document.getElementById('tnSaveBtn'),
  tnCreateBtn: document.getElementById('tnCreateBtn'),
  copiesState: document.getElementById('copiesState'),
  copiesWrap: document.getElementById('copiesWrap'),
  copiesList: document.getElementById('copiesList'),
  copiesMsg: document.getElementById('copiesMsg'),
  copiesForm: document.getElementById('copiesForm'),
  cpInput: document.getElementById('cpInput'),
  cpSaveBtn: document.getElementById('cpSaveBtn'),
  imState: document.getElementById('imState'),
  imMsg: document.getElementById('imMsg'),
  imInput: document.getElementById('imInput'),
  importForm: document.getElementById('importForm'),
  imBtn: document.getElementById('imBtn'),
  svReloadBtn: document.getElementById('svReloadBtn'),
  serverState: document.getElementById('serverState'),
  serverWrap: document.getElementById('serverWrap'),
  svHelper: document.getElementById('svHelper'),
  svApp: document.getElementById('svApp'),
  svDb: document.getElementById('svDb'),
  svCf: document.getElementById('svCf'),
  svHelperNote: document.getElementById('svHelperNote'),
  svServiceBtn: document.getElementById('svServiceBtn'),
  svLogLines: document.getElementById('svLogLines'),
  svLogBtn: document.getElementById('svLogBtn'),
  svBackupBtn: document.getElementById('svBackupBtn'),
  svRestartBtn: document.getElementById('svRestartBtn'),
  svDoctorBtn: document.getElementById('svDoctorBtn'),
  svTnStatusBtn: document.getElementById('svTnStatusBtn'),
  svTnRestartBtn: document.getElementById('svTnRestartBtn'),
  svTnForm: document.getElementById('svTnForm'),
  svTnToken: document.getElementById('svTnToken'),
  svTnInstallBtn: document.getElementById('svTnInstallBtn'),
  svTnLogoutForm: document.getElementById('svTnLogoutForm'),
  svTnReset: document.getElementById('svTnReset'),
  svTnLogoutBtn: document.getElementById('svTnLogoutBtn'),
  svMsg: document.getElementById('svMsg'),
  svOutputMeta: document.getElementById('svOutputMeta'),
  svOutput: document.getElementById('svOutput'),
  usReloadBtn: document.getElementById('usReloadBtn'),
  usState: document.getElementById('usState'),
  usWrap: document.getElementById('usWrap'),
  usBody: document.getElementById('usBody'),
  usMsg: document.getElementById('usMsg'),
  usForm: document.getElementById('usForm'),
  usEmail: document.getElementById('usEmail'),
  usPassword: document.getElementById('usPassword'),
  usRole: document.getElementById('usRole'),
  usAddBtn: document.getElementById('usAddBtn'),
  nmReloadBtn: document.getElementById('nmReloadBtn'),
  nmState: document.getElementById('nmState'),
  nmForm: document.getElementById('nmForm'),
  nmTemplate: document.getElementById('nmTemplate'),
  nmSeparator: document.getElementById('nmSeparator'),
  nmNumMin: document.getElementById('nmNumMin'),
  nmNumMax: document.getElementById('nmNumMax'),
  nmNumProb: document.getElementById('nmNumProb'),
  nmFirst: document.getElementById('nmFirst'),
  nmSecond: document.getElementById('nmSecond'),
  nmSaveBtn: document.getElementById('nmSaveBtn'),
  nmMsg: document.getElementById('nmMsg'),
  dgReloadBtn: document.getElementById('dgReloadBtn'),
  dgFetched: document.getElementById('dgFetched'),
  dgState: document.getElementById('dgState'),
  dgWrap: document.getElementById('dgWrap'),
  dgHelper: document.getElementById('dgHelper'),
  dgApp: document.getElementById('dgApp'),
  dgDb: document.getElementById('dgDb'),
  dgCf: document.getElementById('dgCf'),
  dgTunnel: document.getElementById('dgTunnel'),
  dgCfDomains: document.getElementById('dgCfDomains'),
  dgMsg: document.getElementById('dgMsg'),
};

let lastFocus = null;

function node(tag, className, text) {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text !== undefined && text !== null) el.textContent = String(text);
  return el;
}

function apiFailure(err) {
  if (!err) return { message: 'Permintaan gagal', detail: '' };
  if (typeof err === 'string') return { message: err, detail: '' };
  return {
    message: err.error || 'Permintaan gagal',
    detail: err.detail || '',
  };
}

// Wraps API.request so every call has its own deadline: a stuck request always ends in the
// error state instead of leaving the section spinning. The thrown object carries the HTTP status
// so callers can tell 501 (helper absent) and 504 (helper silent) apart from generic failures.
async function apiRequest(base, path, options) {
  const controller = new AbortController();
  const timer = setTimeout(function () { controller.abort(); }, REQUEST_TIMEOUT_MS);
  let res;
  try {
    res = await API.request(base + path, Object.assign({}, options || {}, { signal: controller.signal }));
  } catch (err) {
    clearTimeout(timer);
    if (err && err.name === 'AbortError') {
      throw { error: 'Server tidak menjawab dalam 30 detik', detail: 'Permintaan ke ' + base + path + ' dibatalkan' };
    }
    throw { error: 'Gagal menghubungi server', detail: String((err && err.message) || err) };
  }
  clearTimeout(timer);
  if (!res) throw { error: 'Sesi berakhir', detail: 'Token tidak valid atau kedaluwarsa' };
  const text = await res.text();
  let body = null;
  if (text) {
    try { body = JSON.parse(text); } catch (parseErr) { body = null; }
  }
  if (!res.ok) {
    throw {
      error: (body && body.error) || ('HTTP ' + res.status),
      detail: (body && body.detail) || '',
      status: res.status,
      requestId: (body && (body.request_id || body.id)) || '',
    };
  }
  return body;
}

function cf(path, options) {
  return apiRequest(CF_BASE, path, options);
}

function apiGet(path) {
  return apiRequest(SYS_BASE, path, { method: 'GET' });
}

function apiPost(path, payload) {
  return apiRequest(SYS_BASE, path, { method: 'POST', body: JSON.stringify(payload || {}) });
}

function slot(stateEl, wrapEl) {
  return {
    loading: function (text) {
      wrapEl.hidden = true;
      stateEl.hidden = false;
      stateEl.className = 'state loading';
      stateEl.replaceChildren(node('span', 'spinner'), node('span', 'state-text', text));
    },
    empty: function (text) {
      wrapEl.hidden = true;
      stateEl.hidden = false;
      stateEl.className = 'state empty';
      stateEl.replaceChildren(node('span', 'state-text', text));
    },
    error: function (err) {
      const info = apiFailure(err);
      wrapEl.hidden = true;
      stateEl.hidden = false;
      stateEl.className = 'state error';
      const body = node('div', 'state-body');
      body.append(node('div', 'state-text', info.message));
      if (info.detail) body.append(node('div', 'state-detail', info.detail));
      stateEl.replaceChildren(body);
    },
    ok: function () {
      stateEl.hidden = true;
      wrapEl.hidden = false;
    },
  };
}

function message(el, kind, text, detail) {
  el.className = 'out ' + kind;
  el.replaceChildren(node('span', 'out-text', text));
  if (detail) el.append(node('span', 'out-detail', detail));
}

function clearMessage(el) {
  el.className = 'out';
  el.replaceChildren();
}

function busy(btn, on) {
  btn.disabled = on;
  btn.setAttribute('aria-busy', on ? 'true' : 'false');
}

async function run(btn, msgEl, fn) {
  busy(btn, true);
  clearMessage(msgEl);
  try {
    await fn();
  } catch (err) {
    const info = apiFailure(err);
    message(msgEl, 'error', info.message, info.detail);
  } finally {
    busy(btn, false);
  }
}

function cell(label, value, className) {
  const td = node('td', className || null, value);
  td.dataset.label = label;
  return td;
}

function actionCell(children) {
  const td = node('td');
  td.dataset.label = 'Actions';
  const box = node('div', 'row-actions');
  children.forEach(function (child) { box.append(child); });
  td.append(box);
  return td;
}

// Token Cloudflare ditulis satu arah: hanya dikirim, tidak pernah ditampilkan kembali.
function tokenCell(profile) {
  const td = node('td');
  td.dataset.label = 'Token';
  td.append(node('span', 'badge' + (profile.has_token ? ' on' : ' off'), profile.has_token ? 'token aktif' : 'belum ada token'));
  const input = document.createElement('input');
  input.type = 'password';
  input.className = 'token-input';
  input.autocomplete = 'off';
  input.spellcheck = false;
  input.placeholder = profile.has_token ? 'ganti API token' : 'tempel API token';
  const save = button(profile.has_token ? 'Ganti token' : 'Simpan token', 'btn btn-p small');
  save.addEventListener('click', function () {
    const value = input.value.trim();
    if (value.length < 20) {
      message(ui.profilesMsg, 'error', 'API token terlalu pendek, salin ulang dari dashboard Cloudflare');
      return;
    }
    run(save, ui.profilesMsg, async function () {
      input.value = '';
      await cf('/profiles/' + encodeURIComponent(profile.id), { method: 'PUT', body: JSON.stringify({ token: value }) });
      await loadProfiles();
      message(ui.profilesMsg, 'ok', 'API token profil ' + profile.id + ' disimpan lewat API token, tanpa OAuth',
        'izin yang dibutuhkan: Zone:Read, Email Routing Rules:Edit, Workers Scripts:Edit');
    });
  });
  td.append(input, save);
  return td;
}

function button(label, className) {
  const btn = node('button', className || 'btn btn-s small', label);
  btn.type = 'button';
  return btn;
}

function badge(text, on) {
  return node('span', 'badge' + (on ? ' on' : ''), text);
}

async function copyText(text) {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (err) { /* clipboard API rejected (http origin): fall through to the textarea copy */ }
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.top = '-1000px';
  document.body.append(area);
  area.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch (err) { ok = false; }
  area.remove();
  return ok;
}

function copyButton(text, label) {
  const btn = button(label || 'Copy', 'btn btn-s small');
  btn.addEventListener('click', async function () {
    const ok = await copyText(text);
    btn.textContent = ok ? 'Copied' : 'Copy gagal';
    setTimeout(function () { btn.textContent = label || 'Copy'; }, 1500);
  });
  return btn;
}

function openModal(title, content, trigger) {
  ui.modalTitle.textContent = title;
  ui.modalBody.replaceChildren(content);
  lastFocus = trigger || document.activeElement;
  ui.modal.hidden = false;
  ui.modalClose.focus();
}

function closeModal() {
  if (ui.modal.hidden) return;
  ui.modal.hidden = true;
  ui.modalBody.replaceChildren();
  if (lastFocus && typeof lastFocus.focus === 'function') lastFocus.focus();
  lastFocus = null;
}

// --- profiles ---

function profileTable() {
  return slot(ui.profilesState, ui.profilesWrap);
}

async function loadProfiles() {
  const s = profileTable();
  ui.profilesBody.replaceChildren();
  s.loading('Memuat profil Cloudflare...');
  try {
    const profiles = await cf('/profiles');
    state.profiles = Array.isArray(profiles) ? profiles : [];
    renderProfiles();
    renderProfileOptions();
  } catch (err) {
    s.error(err);
  }
}

function renderProfiles() {
  const s = profileTable();
  if (state.profiles.length === 0) {
    s.empty('Belum ada profil Cloudflare. Tambahkan profil lewat form di bawah.');
    return;
  }
  s.ok();
  state.profiles.forEach(function (p) {
    const tr = node('tr');
    tr.append(cell('ID', p.id, 'mono'));
    tr.append(cell('Label', p.label || '-'));
    tr.append(cell('Account ID', p.account_id || '-', 'mono'));
    tr.append(cell('Active', p.active ? 'aktif' : 'tidak'));
    tr.append(cell('Worker', p.worker_deployed ? 'deployed' : 'belum'));
    tr.append(cell('Domains', String(p.domain_count === undefined ? '-' : p.domain_count)));
    tr.append(tokenCell(p));
    const activate = button('Activate', 'btn btn-s small');
    activate.disabled = !!p.active;
    activate.addEventListener('click', function () {
      run(activate, ui.profilesMsg, async function () {
        const out = await cf('/profiles/' + encodeURIComponent(p.id) + '/activate', { method: 'POST', body: '{}' });
        await loadProfiles();
        message(ui.profilesMsg, 'ok', 'Profil aktif: ' + ((out && out.active_profile) || p.id));
      });
    });
    const deploy = button('Deploy Worker', 'btn btn-p small');
    deploy.addEventListener('click', function () {
      run(deploy, ui.profilesMsg, async function () {
        const out = await cf('/profiles/' + encodeURIComponent(p.id) + '/deploy-worker', { method: 'POST', body: '{}' });
        await loadProfiles();
        message(ui.profilesMsg, 'ok', 'Worker ' + ((out && out.worker) || state.workerName || p.id) + ' terunggah',
          'modified_on ' + ((out && out.modified_on) || '-') + ', secrets ' + (((out && out.secrets) || []).join(', ') || '-'));
      });
    });
    const dropWorker = button('Delete Worker', 'btn btn-danger small');
    dropWorker.disabled = !p.worker_deployed;
    dropWorker.addEventListener('click', function () {
      const workerName = state.workerName || p.id;
      if (!window.confirm('Hapus Worker ' + workerName + ' pada profil ' + p.id + '?')) return;
      run(dropWorker, ui.profilesMsg, async function () {
        const out = await cf('/profiles/' + encodeURIComponent(p.id) + '/worker', { method: 'DELETE' });
        await loadProfiles();
        message(ui.profilesMsg, 'ok', 'Worker ' + (out && out.worker ? out.worker : workerName) + ' terhapus dari Cloudflare');
      });
    });
    const drop = button('Hapus profil', 'btn btn-danger small');
    drop.addEventListener('click', function () {
      if (!window.confirm('Hapus profil ' + p.id + ' (' + (p.label || p.id) + ')?')) return;
      run(drop, ui.profilesMsg, async function () {
        await cf('/profiles/' + encodeURIComponent(p.id), { method: 'DELETE' });
        await loadProfiles();
        message(ui.profilesMsg, 'ok', 'Profil ' + p.id + ' terhapus dari database');
      });
    });
    tr.append(actionCell([activate, deploy, dropWorker, drop]));
    ui.profilesBody.append(tr);
  });
}

function renderProfileOptions() {
  const keep = ui.dmProfile.value;
  ui.dmProfile.replaceChildren();
  const auto = node('option', null, 'profil aktif');
  auto.value = '';
  ui.dmProfile.append(auto);
  state.profiles.forEach(function (p) {
    const opt = node('option', null, p.id + ' - ' + (p.label || p.id));
    opt.value = p.id;
    ui.dmProfile.append(opt);
  });
  if (keep && state.profiles.some(function (p) { return p.id === keep; })) ui.dmProfile.value = keep;
}

async function testConnection(btn) {
  busy(btn, true);
  clearMessage(ui.statusOut);
  try {
    const status = await cf('/status');
    state.workerName = status.worker_name || '';
    state.workerCompatDate = status.worker_compat_date || '';
    const active = (status.profiles || []).filter(function (p) { return p.active; })[0];
    const line = active
      ? 'Profil aktif: ' + active.id + ' (' + (active.label || active.id) + '), account ' + active.account_id + ', token ' + (active.has_token ? 'ada' : 'tidak ada')
      : 'Tidak ada profil aktif. Aktifkan satu profil dulu sebelum deploy Worker.';
    message(ui.statusOut, 'ok', line,
      'api_base ' + status.api_base + ', profil ' + (status.profiles || []).length + ', domain ' + (status.domains || []).length +
      ', worker ' + status.worker_name + ' (' + status.worker_compat_date + '), tunnel ' + (status.tunnel ? status.tunnel.status : '-'));
  } catch (err) {
    const info = apiFailure(err);
    message(ui.statusOut, 'error', info.message, info.detail);
  } finally {
    busy(btn, false);
  }
}

// --- domains ---

function domainTable() {
  return slot(ui.domainsState, ui.domainsWrap);
}

async function loadDomains() {
  const s = domainTable();
  ui.domainsBody.replaceChildren();
  s.loading('Memuat domain...');
  try {
    const domains = await cf('/domains');
    state.domains = Array.isArray(domains) ? domains : [];
    renderDomains();
  } catch (err) {
    s.error(err);
  }
}

function renderDomains() {
  const s = domainTable();
  if (state.domains.length === 0) {
    s.empty('Belum ada domain terdaftar. Tambahkan domain lewat form di bawah.');
    return;
  }
  s.ok();
  state.domains.forEach(function (d) {
    const tr = node('tr');
    tr.append(cell('Domain', d.domain, 'mono'));
    tr.append(cell('Profile', d.profile || '-'));
    tr.append(cell('Zone ID', d.zone_id || '-', 'mono'));
    const routingCell = node('td');
    routingCell.dataset.label = 'Routing';
    routingCell.append(badge(d.routing ? 'ON' : 'OFF', !!d.routing));
    tr.append(routingCell);
    const unlockCell = node('td');
    unlockCell.dataset.label = 'Unlock DNS';
    const unlockWrap = node('label', 'check');
    const unlock = node('input');
    unlock.type = 'checkbox';
    unlock.setAttribute('aria-label', 'unlock DNS ' + d.domain);
    unlockWrap.append(unlock, node('span', null, 'unlock DNS'));
    unlockCell.append(unlockWrap);
    tr.append(unlockCell);
    const toggle = button(d.routing ? 'Disable routing' : 'Enable routing', 'btn btn-s small');
    toggle.addEventListener('click', function () {
      const want = !d.routing;
      if (want && unlock.checked && !window.confirm('Aktifkan routing ' + d.domain + ' dan buka DNS routing di Cloudflare?')) return;
      run(toggle, ui.domainsMsg, async function () {
        const out = await cf('/domains/' + encodeURIComponent(d.domain) + '/routing', {
          method: 'POST',
          body: JSON.stringify({ routing: want, unlock_dns: !!unlock.checked }),
        });
        await loadDomains();
        message(ui.domainsMsg, 'ok',
          d.domain + ': routing ' + (out.routing ? 'ON' : 'OFF') + (out.dns_unlocked ? ', DNS routing dibuka' : ', DNS routing tidak diubah'));
      });
    });
    const drop = button('Hapus domain', 'btn btn-danger small');
    drop.addEventListener('click', function () {
      if (!window.confirm('Hapus domain ' + d.domain + ' dari panel? Routing Cloudflare tidak diubah.')) return;
      run(drop, ui.domainsMsg, async function () {
        await cf('/domains/' + encodeURIComponent(d.domain), { method: 'DELETE' });
        await loadDomains();
        message(ui.domainsMsg, 'ok', 'Domain ' + d.domain + ' terhapus dari database, Cloudflare tidak diubah');
      });
    });
    tr.append(actionCell([toggle, drop]));
    ui.domainsBody.append(tr);
  });
}

// --- tunnel ---

function tunnelSlot() {
  return slot(ui.tunnelState, ui.tunnelWrap);
}

async function loadTunnel() {
  const s = tunnelSlot();
  s.loading('Memuat status tunnel...');
  clearMessage(ui.tunnelMsg);
  try {
    const tunnel = await cf('/tunnel');
    state.tunnel = tunnel;
    renderTunnel();
  } catch (err) {
    s.error(err);
  }
}

function renderTunnel() {
  const s = tunnelSlot();
  const t = state.tunnel;
  if (!t) {
    s.empty('Belum ada data tunnel. Simpan nama tunnel lewat form di bawah.');
    return;
  }
  s.ok();
  ui.tunnelInfo.replaceChildren();
  const rows = [
    ['Nama', t.name || '-'],
    ['ID', t.id || '-'],
    ['Status', t.status || 'unknown'],
    ['Connections', String(t.connections === undefined ? 0 : t.connections)],
    ['Web host', t.web_host || '-'],
    ['Sumber', t.source || '-'],
    ['Account ID', t.account_id || '-'],
  ];
  rows.forEach(function (row) {
    ui.tunnelInfo.append(node('dt', null, row[0]), node('dd', 'mono', row[1]));
  });
  if (!ui.tnName.value) ui.tnName.value = t.name || '';
  if (!ui.tnWebHost.value) ui.tnWebHost.value = t.web_host || '';
  if (t.error) message(ui.tunnelMsg, 'error', 'Cloudflare: ' + t.error);
}

function tunnelPayload() {
  return {
    web_host: ui.tnWebHost.value.trim(),
    name: ui.tnName.value.trim(),
  };
}

function tunnelResultNode(out) {
  const box = node('div');
  const kv = node('dl', 'kv');
  [['ID', out.id], ['Nama', out.name]].forEach(function (row) {
    kv.append(node('dt', null, row[0]), node('dd', 'mono', row[1]));
  });
  box.append(kv);
  box.append(node('h3', null, 'Connector token'));
  const tokenBlock = node('code', 'block', out.token);
  box.append(tokenBlock);
  const tokenRow = node('div', 'row-actions');
  tokenRow.append(copyButton(out.token, 'Copy token'));
  box.append(tokenRow);
  box.append(node('h3', null, 'Perintah root'));
  const commands = out.commands || [];
  if (commands.length === 0) {
    box.append(node('p', 'note', 'Tidak ada perintah yang dikembalikan API.'));
  } else {
    commands.forEach(function (cmd) {
      box.append(node('code', 'block', cmd));
      const row = node('div', 'row-actions');
      row.append(copyButton(cmd, 'Copy perintah'));
      box.append(row);
    });
  }
  return box;
}

// --- server (root helper) ---

function systemSlot() {
  return slot(ui.serverState, ui.serverWrap);
}

function helperButtons() {
  return [
    ui.svServiceBtn, ui.svLogBtn, ui.svBackupBtn, ui.svRestartBtn, ui.svDoctorBtn,
    ui.svTnStatusBtn, ui.svTnInstallBtn, ui.svTnRestartBtn, ui.svTnLogoutBtn,
  ];
}

// Every action in this section is executed by the root helper, so the buttons stay disabled until
// /status confirms the helper is installed, and go back to disabled as soon as a 501 says it is not.
function applyHelperGate() {
  const missing = state.systemHelper === false;
  const unknown = state.systemHelper !== true;
  let title = '';
  if (missing) title = 'Butuh helper root: pasang lewat deploy/linux/install.sh di server';
  else if (unknown) title = 'Status helper root belum diketahui, tekan Muat ulang dulu';
  helperButtons().forEach(function (btn) {
    if (unknown) btn.disabled = true;
    else if (btn.getAttribute('aria-busy') !== 'true') btn.disabled = false;
    btn.title = title;
  });
  ui.svHelperNote.hidden = !unknown;
  if (missing) {
    ui.svHelperNote.textContent = 'Fitur service, log, backup, dan tunnel butuh helper root. Jalankan deploy/linux/install.sh di server untuk memasangnya, lalu tekan Muat ulang.';
  } else if (unknown) {
    ui.svHelperNote.textContent = 'Status helper root belum diketahui. Tekan Muat ulang untuk memuatnya.';
  }
}

function formatUptime(seconds) {
  const total = Math.max(0, Math.floor(Number(seconds) || 0));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const parts = [];
  if (days) parts.push(days + ' hari');
  if (hours) parts.push(hours + ' jam');
  if (minutes) parts.push(minutes + ' menit');
  if (!days && !hours) parts.push(secs + ' detik');
  return parts.join(' ');
}

function formatBytes(bytes) {
  const n = Number(bytes);
  if (!isFinite(n) || n <= 0) return '0 KB';
  if (n < 1024) return n + ' byte';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1048576).toFixed(2) + ' MB';
}

function formatMb(bytes) {
  const n = Number(bytes);
  if (!isFinite(n) || n <= 0) return '0 MB';
  return (n / 1048576).toFixed(1) + ' MB';
}

function countOf(value) {
  return value === undefined || value === null ? 0 : value;
}

function fillKv(dl, rows) {
  dl.replaceChildren();
  rows.forEach(function (row) {
    dl.append(node('dt', null, row[0]), node('dd', 'mono', row[1]));
  });
}

function renderSystem() {
  const s = systemSlot();
  const out = state.system || {};
  const helper = out.helper || {};
  const app = out.app || {};
  const db = out.db || {};
  const cloudflare = out.cloudflare || {};
  s.ok();
  fillKv(ui.svHelper, [
    ['Terpasang', helper.installed ? 'terpasang' : 'belum terpasang'],
    ['Direktori', helper.dir || '-'],
    ['Request tertunda', String(countOf(helper.pending))],
  ]);
  fillKv(ui.svApp, [
    ['Uptime', formatUptime(app.uptime_seconds)],
    ['RAM', formatMb(app.rss_bytes)],
    ['Versi Go', app.go_version || '-'],
    ['Site name', app.site_name || '-'],
    ['Port', app.port || '-'],
  ]);
  fillKv(ui.svDb, [
    ['Ukuran file', formatBytes(db.bytes) + ' (' + countOf(db.bytes) + ' byte)'],
    ['Path', db.path || '-'],
    ['Inbox', String(countOf(db.inboxes))],
    ['Pesan', String(countOf(db.messages))],
    ['Lampiran', String(countOf(db.attachments))],
  ]);
  fillKv(ui.svCf, [
    ['Profil', String(countOf(cloudflare.profiles))],
    ['Domain', String(countOf(cloudflare.domains))],
    ['Profil aktif', cloudflare.active_profile || '-'],
    ['Worker', cloudflare.worker_name || '-'],
  ]);
}

function enterHelperMissing(err) {
  state.systemHelper = false;
  const info = apiFailure(err);
  const parts = [];
  if (info.detail) parts.push(info.detail);
  parts.push('deploy/linux/install.sh memasang helper root di server');
  if (err && err.requestId) parts.push('request id ' + err.requestId);
  systemSlot().error({ error: 'Helper root belum terpasang', detail: parts.join(' | ') });
}

// The server names the failing helper request in the body; keep that id visible in every error.
function withRequestId(err) {
  if (!err || !err.requestId) return err;
  const info = apiFailure(err);
  return {
    error: info.message,
    detail: (info.detail ? info.detail + ' | ' : '') + 'request id ' + err.requestId,
  };
}

async function loadSystemStatus() {
  const s = systemSlot();
  s.loading('Memuat status server...');
  try {
    const out = await apiGet('/status');
    if (!out) {
      state.system = null;
      state.systemHelper = null;
      s.empty('API tidak mengembalikan status server. Tekan Muat ulang untuk mencoba lagi.');
      return;
    }
    state.system = out;
    state.systemHelper = !!(state.system.helper && state.system.helper.installed);
    renderSystem();
  } catch (err) {
    if (err && err.status === 501) {
      enterHelperMissing(err);
    } else {
      state.systemHelper = null;
      s.error(withRequestId(err));
    }
  } finally {
    applyHelperGate();
  }
}

function renderActionOutput(out) {
  const body = (out && typeof out === 'object') ? out : {};
  const status = body.status || 'unknown';
  const seconds = typeof body.seconds === 'number' ? body.seconds : null;
  let meta = 'status: ' + status;
  if (seconds !== null) meta += ', ' + seconds.toFixed(2) + ' detik';
  if (body.error) meta += ', error: ' + body.error;
  message(ui.svOutputMeta, body.ok === false ? 'error' : 'ok', meta);
  const output = body.output ? String(body.output) : '';
  ui.svOutput.textContent = output;
  ui.svOutput.hidden = !output;
  return body;
}

function announceAction(body, okText) {
  const status = (body && body.status) || 'unknown';
  if (body && body.ok === false) {
    message(ui.svMsg, 'error', body.error || 'Aksi ditolak helper', 'status: ' + status);
    return;
  }
  message(ui.svMsg, 'ok', okText, 'status: ' + status);
}

function handleSystemError(err) {
  if (err && err.status === 501) {
    enterHelperMissing(err);
    message(ui.svMsg, 'error', 'Helper root belum terpasang', 'deploy/linux/install.sh memasang helper root, lalu tekan Muat ulang');
    return;
  }
  if (err && err.status === 504) {
    message(ui.svMsg, 'error', 'Helper tidak menjawab dalam batas waktu',
      err.requestId ? 'request id ' + err.requestId : 'Periksa service tempmail-root dan journald di server');
    return;
  }
  const info = apiFailure(withRequestId(err));
  message(ui.svMsg, 'error', info.message, info.detail);
}

function clearActionOutput() {
  clearMessage(ui.svOutputMeta);
  ui.svOutput.textContent = '';
  ui.svOutput.hidden = true;
}

async function runAction(btn, fn) {
  busy(btn, true);
  clearMessage(ui.svMsg);
  clearActionOutput();
  try {
    await fn();
  } catch (err) {
    handleSystemError(err);
  } finally {
    busy(btn, false);
    applyHelperGate();
  }
}

function sleep(ms) {
  return new Promise(function (resolve) { setTimeout(resolve, ms); });
}

// /health is public and served by the app itself, so it is polled with a bare fetch: the app
// answering it again is how we know the restart finished.
async function waitForHealth(timeoutMs, intervalMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await sleep(intervalMs);
    try {
      const res = await fetch('/health', { cache: 'no-store' });
      if (res.ok) return true;
    } catch (err) { /* app masih mati, coba lagi sampai tenggat */ }
  }
  return false;
}

function bindServer() {
  ui.svReloadBtn.addEventListener('click', function () {
    run(ui.svReloadBtn, ui.svMsg, loadSystemStatus);
  });

  ui.svServiceBtn.addEventListener('click', function () {
    runAction(ui.svServiceBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'service-status' }));
      announceAction(body, 'Status service tempmail, cloudflared, dan timer backup diambil');
    });
  });

  ui.svLogBtn.addEventListener('click', function () {
    const lines = parseInt(ui.svLogLines.value, 10) || 100;
    runAction(ui.svLogBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'logs', lines: lines }));
      announceAction(body, 'Log service tempmail ' + lines + ' baris terakhir diambil');
    });
  });

  ui.svBackupBtn.addEventListener('click', function () {
    if (!window.confirm('Jalankan backup sekarang: systemctl start tempmail-backup.service di server?')) return;
    runAction(ui.svBackupBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'backup-now' }));
      announceAction(body, 'Backup tempmail dijalankan lewat helper root');
    });
  });

  ui.svRestartBtn.addEventListener('click', function () {
    if (!window.confirm('Restart service tempmail sekarang? Service tempmail berhenti sejenak dan halaman ini akan terputus sampai service kembali aktif.')) return;
    runAction(ui.svRestartBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'restart-service' }));
      if (!body || body.status !== 'accepted') {
        announceAction(body, 'Permintaan restart service tempmail dikirim');
        return;
      }
      message(ui.svMsg, 'ok', 'Restart service tempmail diterima, menunggu service kembali aktif...');
      const alive = await waitForHealth(HEALTH_TIMEOUT_MS, HEALTH_INTERVAL_MS);
      if (alive) {
        message(ui.svMsg, 'ok', 'service kembali aktif');
      } else {
        message(ui.svMsg, 'error', 'Service belum kembali aktif', 'Tidak ada balasan dari /health dalam 30 detik setelah restart');
      }
    });
  });

  ui.svDoctorBtn.addEventListener('click', function () {
    runAction(ui.svDoctorBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'doctor' }));
      announceAction(body, 'Doctor selesai memeriksa helper, env, data dir, dan cloudflared');
    });
  });

  ui.svTnStatusBtn.addEventListener('click', function () {
    runAction(ui.svTnStatusBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'tunnel-status' }));
      announceAction(body, 'Status tunnel Cloudflare diambil');
    });
  });

  ui.svTnForm.addEventListener('submit', function (event) {
    event.preventDefault();
    const token = ui.svTnToken.value.trim();
    ui.svTnToken.value = '';
    if (!token) {
      message(ui.svMsg, 'error', 'Token Cloudflare kosong', 'Salin connector token dari dashboard Cloudflare dulu');
      return;
    }
    if (!window.confirm('Pasang atau update service cloudflared di server dengan token ini?')) return;
    runAction(ui.svTnInstallBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'tunnel-install', token: token }));
      announceAction(body, 'Service cloudflared dipasang atau diperbarui');
    });
  });

  ui.svTnRestartBtn.addEventListener('click', function () {
    if (!window.confirm('Restart service cloudflared sekarang? Koneksi tunnel ke Cloudflare terputus sementara.')) return;
    runAction(ui.svTnRestartBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'tunnel-restart' }));
      announceAction(body, 'Service cloudflared direstart');
    });
  });

  ui.svTnLogoutForm.addEventListener('submit', function (event) {
    event.preventDefault();
    const typed = ui.svTnReset.value.trim();
    if (typed !== 'RESET') {
      message(ui.svMsg, 'error', 'Konfirmasi logout belum cocok', 'Ketik RESET di kolom konfirmasi untuk melanjutkan');
      return;
    }
    if (!window.confirm('Logout tunnel: hapus credential Cloudflare dan service cloudflared di server (/etc/cloudflared dan /root/.cloudflared)? Setelah ini cloudflared harus dipasang ulang.')) return;
    runAction(ui.svTnLogoutBtn, async function () {
      const body = renderActionOutput(await apiPost('/action', { action: 'tunnel-logout', confirm: 'RESET' }));
      ui.svTnReset.value = '';
      if (body && body.ok === false) {
        message(ui.svMsg, 'error', body.error || 'Logout tunnel ditolak helper', 'status: ' + (body.status || 'unknown'));
        return;
      }
      message(ui.svMsg, 'ok', 'Credential Cloudflare dihapus dan service cloudflared dilepas dari server',
        'cloudflared harus dipasang lagi lewat Pasang/Update tunnel');
    });
  });
}

// --- copies ---

function copiesSlot() {
  return slot(ui.copiesState, ui.copiesWrap);
}

async function loadCopies() {
  const s = copiesSlot();
  s.loading('Memuat tujuan salinan...');
  try {
    const out = await cf('/copies');
    state.copies = (out && out.destinations) || [];
    ui.cpInput.value = state.copies.join(', ');
    renderCopies();
  } catch (err) {
    s.error(err);
  }
}

function renderCopies() {
  const s = copiesSlot();
  ui.copiesList.replaceChildren();
  if (state.copies.length === 0) {
    s.empty('Belum ada tujuan salinan. Kosong berarti email tidak disalin ke mana pun.');
    return;
  }
  s.ok();
  state.copies.forEach(function (dest) {
    const li = node('li');
    li.append(node('span', 'mono', dest));
    li.append(copyButton(dest, 'Copy'));
    ui.copiesList.append(li);
  });
}

// --- users (akun panel) ---

const USERS_BASE = '/api/admin/users';
const NAMING_BASE = '/api/admin/naming-rules';

function usersReq(path, options) {
  return apiRequest(USERS_BASE, path, options);
}

function namingReq(options) {
  return apiRequest(NAMING_BASE, '', options);
}

function formatWaktu(value) {
  if (!value) return '-';
  const d = new Date(value);
  if (isNaN(d.getTime())) return String(value);
  return d.toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
}

// Identitas pemakai panel ada di localStorage.auth_user (ditulis app.js saat login).
function currentUser() {
  try {
    const parsed = JSON.parse(localStorage.getItem('auth_user') || '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (err) {
    return {};
  }
}

function isSelfUser(user, me) {
  if (!user || !me) return false;
  if (me.id !== undefined && me.id !== null && user.id !== undefined && user.id !== null) {
    return Number(me.id) === Number(user.id);
  }
  return Boolean(me.email) && me.email === user.email;
}

function usersSlot() {
  return slot(ui.usState, ui.usWrap);
}

async function loadUsers() {
  const s = usersSlot();
  ui.usBody.replaceChildren();
  s.loading('Memuat pengguna...');
  try {
    const users = await usersReq('', { method: 'GET' });
    state.users = Array.isArray(users) ? users : [];
    renderUsers();
  } catch (err) {
    s.error(err);
  }
}

function userRow(user, me) {
  const tr = node('tr');
  const self = isSelfUser(user, me);

  const emailCell = cell('Email', user.email, 'mono');
  if (self) {
    emailCell.append(node('span', 'badge', 'akun sendiri'));
  }
  tr.append(emailCell);

  const roleCell = node('td');
  roleCell.dataset.label = 'Role';
  roleCell.append(badge(user.role === 'admin' ? 'admin' : 'user', user.role === 'admin'));
  tr.append(roleCell);

  tr.append(cell('Dibuat', user.created_at ? formatWaktu(user.created_at) : '-'));

  const roleBox = node('td');
  roleBox.dataset.label = 'Ganti role';
  const roleSelect = node('select');
  roleSelect.setAttribute('aria-label', 'Role baru untuk ' + user.email);
  ['user', 'admin'].forEach(function (value) {
    const opt = node('option', null, value);
    opt.value = value;
    if (user.role === value) opt.selected = true;
    roleSelect.append(opt);
  });
  const saveRole = button('Simpan role', 'btn btn-s small');
  saveRole.addEventListener('click', function () {
    const next = roleSelect.value;
    if (next === user.role) {
      message(ui.usMsg, 'error', 'Role ' + user.email + ' sudah ' + user.role, 'Pilih role lain dulu sebelum menyimpan');
      return;
    }
    run(saveRole, ui.usMsg, async function () {
      // Handler Go menuntut email dan role sekaligus (binding required), jadi email ikut dikirim apa adanya.
      await usersReq('/' + user.id, { method: 'PUT', body: JSON.stringify({ email: user.email, role: next }) });
      await loadUsers();
      message(ui.usMsg, 'ok', 'Role ' + user.email + ' menjadi ' + next);
    });
  });
  roleBox.append(roleSelect, saveRole);
  tr.append(roleBox);

  const pwBox = node('td');
  pwBox.dataset.label = 'Password baru';
  const pwInput = document.createElement('input');
  pwInput.type = 'password';
  pwInput.autocomplete = 'new-password';
  pwInput.spellcheck = false;
  pwInput.placeholder = 'password baru';
  pwInput.setAttribute('aria-label', 'Password baru untuk ' + user.email);
  const resetBtn = button('Reset password', 'btn btn-danger small');
  resetBtn.addEventListener('click', function () {
    // Password ditulis satu arah: dibaca sekali, kolom langsung dikosongkan, tidak pernah ditampilkan lagi.
    const value = pwInput.value;
    pwInput.value = '';
    if (value.length < 6) {
      message(ui.usMsg, 'error', 'Password baru minimal 6 karakter', 'Isi kolom password baru di baris ' + user.email + ', kolom dikosongkan setiap kali tombol ditekan');
      return;
    }
    if (!window.confirm('Ganti password akun ' + user.email + ' (id ' + user.id + ')? Password lama langsung tidak berlaku.')) return;
    run(resetBtn, ui.usMsg, async function () {
      await usersReq('/' + user.id + '/password', { method: 'PUT', body: JSON.stringify({ password: value }) });
      message(ui.usMsg, 'ok', 'Password ' + user.email + ' diganti', 'Serahkan password baru langsung ke pemilik akun, panel tidak menyimpannya');
    });
  });
  pwBox.append(pwInput, resetBtn);
  tr.append(pwBox);

  const drop = button('Hapus pengguna', 'btn btn-danger small');
  if (self) {
    drop.disabled = true;
    drop.title = 'Akun yang sedang dipakai tidak bisa dihapus dari panel';
  }
  drop.addEventListener('click', function () {
    if (self) return;
    if (!window.confirm('Hapus pengguna ' + user.email + ' (id ' + user.id + ')? Akun ini langsung tidak bisa login.')) return;
    run(drop, ui.usMsg, async function () {
      await usersReq('/' + user.id, { method: 'DELETE' });
      await loadUsers();
      message(ui.usMsg, 'ok', 'Pengguna ' + user.email + ' terhapus');
    });
  });
  tr.append(actionCell([drop]));
  return tr;
}

function renderUsers() {
  const s = usersSlot();
  if (state.users.length === 0) {
    s.empty('Belum ada akun pengguna. Tambahkan lewat form di bawah.');
    return;
  }
  s.ok();
  const me = currentUser();
  state.users.forEach(function (user) {
    ui.usBody.append(userRow(user, me));
  });
}

// --- aturan nama ---

function namingSlot() {
  return slot(ui.nmState, ui.nmForm);
}

function namingJsonArray(label, raw) {
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (err) {
    throw { error: label + ' bukan JSON yang valid', detail: err.message };
  }
  if (!Array.isArray(parsed)) {
    throw { error: label + ' harus array JSON', detail: 'Contoh: ["kata","kata lain"]' };
  }
  const words = parsed.map(function (item) { return String(item).trim(); }).filter(Boolean);
  if (words.length === 0) {
    throw { error: label + ' tidak boleh kosong', detail: 'Isi minimal satu kata per elemen' };
  }
  return words;
}

function namingNumber(label, input) {
  const value = parseInt(input.value, 10);
  if (isNaN(value) || value < 0) {
    throw { error: label + ' harus angka 0 atau lebih', detail: 'Perbaiki kolom ' + label };
  }
  return value;
}

function renderNaming(rules) {
  ui.nmTemplate.value = rules.template || '';
  ui.nmSeparator.value = rules.separator || '';
  ui.nmNumMin.value = String(countOf(rules.num_min));
  ui.nmNumMax.value = String(countOf(rules.num_max));
  ui.nmNumProb.value = String(countOf(rules.num_probability));
  ui.nmFirst.value = JSON.stringify(Array.isArray(rules.first_words) ? rules.first_words : [], null, 2);
  ui.nmSecond.value = JSON.stringify(Array.isArray(rules.second_words) ? rules.second_words : [], null, 2);
}

async function loadNaming() {
  const s = namingSlot();
  s.loading('Memuat aturan nama...');
  try {
    const rules = await namingReq({ method: 'GET' });
    if (!rules || typeof rules !== 'object' || Array.isArray(rules)) {
      s.empty('API tidak mengembalikan aturan nama. Muat ulang untuk mencoba lagi.');
      return;
    }
    renderNaming(rules);
    s.ok();
  } catch (err) {
    s.error(err);
  }
}

// --- diagnostik ---

function diagnosticsTunnelRows(tunnel) {
  if (!tunnel) {
    return [['Status', 'tidak diketahui'], ['Nama', '-'], ['Koneksi', '-']];
  }
  return [
    ['Status', tunnel.status || 'unknown'],
    ['Nama', tunnel.name || '-'],
    ['ID', tunnel.id || '-'],
    ['Koneksi', String(countOf(tunnel.connections))],
    ['Web host', tunnel.web_host || '-'],
    ['Sumber', tunnel.source || '-'],
  ];
}

async function loadDiagnostics() {
  const s = slot(ui.dgState, ui.dgWrap);
  s.loading('Memuat diagnostik...');
  clearMessage(ui.dgMsg);
  const results = await Promise.allSettled([apiGet('/status'), cf('/status')]);
  const sys = results[0].status === 'fulfilled' ? results[0].value : null;
  const cloud = results[1].status === 'fulfilled' ? results[1].value : null;
  const stamp = 'Diambil ' + formatWaktu(new Date());
  if (!sys && !cloud) {
    ui.dgFetched.textContent = stamp + ' (gagal)';
    s.error(results[0].reason || results[1].reason);
    return;
  }
  const helper = (sys && sys.helper) || {};
  const app = (sys && sys.app) || {};
  const db = (sys && sys.db) || {};
  const sysCf = (sys && sys.cloudflare) || {};
  s.ok();
  fillKv(ui.dgHelper, [
    ['Terpasang', sys ? (helper.installed ? 'terpasang' : 'belum terpasang') : 'tidak diketahui'],
    ['Direktori', helper.dir || '-'],
    ['Request tertunda', sys ? String(countOf(helper.pending)) : '-'],
  ]);
  fillKv(ui.dgApp, [
    ['Uptime', sys ? formatUptime(app.uptime_seconds) : '-'],
    ['RAM', sys ? formatMb(app.rss_bytes) : '-'],
    ['Versi Go', app.go_version || '-'],
    ['Port', app.port || '-'],
    ['Site name', app.site_name || '-'],
  ]);
  fillKv(ui.dgDb, [
    ['Ukuran file', sys ? formatBytes(db.bytes) + ' (' + countOf(db.bytes) + ' byte)' : '-'],
    ['Path', db.path || '-'],
    ['Inbox', sys ? String(countOf(db.inboxes)) : '-'],
    ['Pesan', sys ? String(countOf(db.messages)) : '-'],
    ['Lampiran', sys ? String(countOf(db.attachments)) : '-'],
  ]);
  const copies = (cloud && Array.isArray(cloud.copies)) ? cloud.copies : [];
  fillKv(ui.dgCf, [
    ['Profil', String(countOf(sysCf.profiles))],
    ['Domain', String(countOf(sysCf.domains))],
    ['Profil aktif', (cloud && cloud.active_profile) || sysCf.active_profile || '-'],
    ['Nama Worker', (cloud && cloud.worker_name) || sysCf.worker_name || '-'],
    ['Tanggal kompatibilitas Worker', (cloud && cloud.worker_compat_date) || '-'],
    ['api_base', (cloud && cloud.api_base) || '-'],
    ['Tujuan salinan', cloud ? String(copies.length) + ' alamat' : '-'],
  ]);
  fillKv(ui.dgTunnel, diagnosticsTunnelRows(cloud && cloud.tunnel));
  ui.dgCfDomains.replaceChildren();
  const domains = (cloud && Array.isArray(cloud.domains)) ? cloud.domains : [];
  if (domains.length === 0) {
    ui.dgCfDomains.append(node('li', null, cloud ? 'Belum ada domain terdaftar di database.' : 'Daftar domain butuh status Cloudflare yang berhasil dimuat.'));
  } else {
    domains.forEach(function (d) {
      const li = node('li');
      li.append(node('span', 'mono', d.domain));
      li.append(badge(d.routing ? 'routing ON' : 'routing OFF', !!d.routing));
      li.append(node('span', null, d.profile ? 'profil ' + d.profile : 'profil aktif'));
      li.append(node('span', null, d.enabled ? 'aktif' : 'nonaktif'));
      ui.dgCfDomains.append(li);
    });
  }
  const failed = [];
  if (!sys) failed.push('status sistem');
  if (!cloud) failed.push('status Cloudflare');
  ui.dgFetched.textContent = stamp + (failed.length ? ' (sebagian gagal: ' + failed.join(', ') + ')' : '');
  if (failed.length) {
    const reason = (!sys && results[0].reason) || (!cloud && results[1].reason);
    const info = apiFailure(reason instanceof Error ? { error: reason.message } : reason);
    message(ui.dgMsg, 'error', 'Sebagian data gagal dimuat: ' + failed.join(' dan '), info.detail || info.message);
  }
}

// --- boot ---

function renderGate(user) {
  ui.panel.hidden = true;
  ui.gate.hidden = false;
  ui.gate.replaceChildren(
    node('div', null, 'Akun ini tidak punya role admin, jadi panel Cloudflare tidak bisa dipakai.'),
    node('div', 'note', 'Login sebagai: ' + ((user && user.email) || 'unknown') + ' (role: ' + ((user && user.role) || '-') + ')')
  );
  const link = node('a', 'btn btn-s small', 'Kembali ke Inbox');
  link.href = '/app/';
  ui.gate.append(link);
}

function bindChrome() {
  ui.logoutBtn.addEventListener('click', function () { API.logout(); });
  ui.modalClose.addEventListener('click', closeModal);
  ui.modal.addEventListener('click', function (event) {
    if (event.target === ui.modal) closeModal();
  });
  document.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') closeModal();
  });
}

function bindSections() {
  ui.testConnBtn.addEventListener('click', function () { testConnection(ui.testConnBtn); });
  ui.pfReloadBtn.addEventListener('click', function () { run(ui.pfReloadBtn, ui.profilesMsg, loadProfiles); });
  ui.dmReloadBtn.addEventListener('click', function () { run(ui.dmReloadBtn, ui.domainsMsg, loadDomains); });
  ui.tnReloadBtn.addEventListener('click', function () { run(ui.tnReloadBtn, ui.tunnelMsg, loadTunnel); });

  ui.profileForm.addEventListener('submit', function (event) {
    event.preventDefault();
    const profileId = ui.pfId.value.trim();
    run(ui.pfAddBtn, ui.profilesMsg, async function () {
      await cf('/profiles', {
        method: 'POST',
        body: JSON.stringify({
          id: profileId,
          label: ui.pfLabel.value.trim(),
          account_id: ui.pfAccount.value.trim(),
          token: ui.pfToken.value.trim(),
          activate: ui.pfActivate.checked,
        }),
      });
      ui.profileForm.reset();
      await loadProfiles();
      message(ui.profilesMsg, 'ok', 'Profil ' + profileId + ' tersimpan');
    });
  });

  ui.domainForm.addEventListener('submit', function (event) {
    event.preventDefault();
    const domain = ui.dmDomain.value.trim();
    run(ui.dmAddBtn, ui.domainsMsg, async function () {
      await cf('/domains', {
        method: 'POST',
        body: JSON.stringify({ domain: domain, profile: ui.dmProfile.value }),
      });
      ui.domainForm.reset();
      await loadDomains();
      message(ui.domainsMsg, 'ok', 'Domain ' + domain + ' ditambahkan, zone ditemukan di profil');
    });
  });

  ui.tunnelForm.addEventListener('submit', function (event) {
    event.preventDefault();
    run(ui.tnSaveBtn, ui.tunnelMsg, async function () {
      const out = await cf('/tunnel', { method: 'PUT', body: JSON.stringify(tunnelPayload()) });
      state.tunnel = out;
      renderTunnel();
      message(ui.tunnelMsg, 'ok', 'Tunnel disimpan: ' + (out.name || '-') + ' / ' + (out.web_host || '-'));
    });
  });

  ui.tnCreateBtn.addEventListener('click', function () {
    const payload = tunnelPayload();
    if (!window.confirm('Buat tunnel baru di Cloudflare dengan nama ' + (payload.name || 'nama tersimpan') + '?')) return;
    run(ui.tnCreateBtn, ui.tunnelMsg, async function () {
      const out = await cf('/tunnel/create', { method: 'POST', body: JSON.stringify(payload) });
      await loadTunnel();
      openModal('Tunnel dibuat', tunnelResultNode(out), ui.tnCreateBtn);
    });
  });

  ui.copiesForm.addEventListener('submit', function (event) {
    event.preventDefault();
    const destinations = ui.cpInput.value.split(',').map(function (part) { return part.trim(); }).filter(Boolean);
    run(ui.cpSaveBtn, ui.copiesMsg, async function () {
      const out = await cf('/copies', { method: 'PUT', body: JSON.stringify({ destinations: destinations }) });
      state.copies = (out && out.destinations) || [];
      ui.cpInput.value = state.copies.join(', ');
      renderCopies();
      const applied = (out && out.applied_to) || [];
      message(ui.copiesMsg, 'ok', 'Tujuan salinan disimpan (' + state.copies.length + ' alamat)',
        applied.length ? 'worker aktif di profil: ' + applied.join(', ') : 'belum ada profil dengan Worker ter-deploy, deploy ulang Worker supaya alamat ini terpakai');
    });
  });

  ui.importForm.addEventListener('submit', function (event) {
    event.preventDefault();
    clearMessage(ui.imMsg);
    let parsed;
    try {
      parsed = JSON.parse(ui.imInput.value);
    } catch (err) {
      message(ui.imMsg, 'error', 'JSON tidak valid', err.message);
      return;
    }
    run(ui.imBtn, ui.imMsg, async function () {
      ui.imState.hidden = false;
      ui.imState.className = 'state loading';
      ui.imState.replaceChildren(node('span', 'spinner'), node('span', 'state-text', 'Mengimpor state...'));
      try {
        const out = await cf('/import', { method: 'POST', body: JSON.stringify({ state: parsed }) });
        const imported = (out && out.imported) || {};
        const count = function (value) { return value === undefined || value === null ? 0 : value; };
        message(ui.imMsg, 'ok',
          'Impor selesai: profil ' + count(imported.profiles) + ', domain ' + count(imported.domains) +
          ', tunnel ' + (imported.tunnel ? 'ya' : 'tidak') + ', profil aktif ' + (imported.active_profile || '-'));
        await Promise.all([loadProfiles(), loadDomains(), loadTunnel(), loadCopies()]);
      } finally {
        ui.imState.hidden = true;
      }
    });
  });

  ui.usReloadBtn.addEventListener('click', function () { run(ui.usReloadBtn, ui.usMsg, loadUsers); });

  ui.usForm.addEventListener('submit', function (event) {
    event.preventDefault();
    clearMessage(ui.usMsg);
    const email = ui.usEmail.value.trim();
    const password = ui.usPassword.value;
    const role = ui.usRole.value;
    // Password ditulis satu arah: dibaca sekali, kolom langsung dikosongkan.
    ui.usPassword.value = '';
    if (password.length < 6) {
      message(ui.usMsg, 'error', 'Password minimal 6 karakter', 'Isi ulang kolom password, kolom dikosongkan setelah setiap kirim');
      return;
    }
    run(ui.usAddBtn, ui.usMsg, async function () {
      await usersReq('', { method: 'POST', body: JSON.stringify({ email: email, password: password, role: role }) });
      ui.usForm.reset();
      await loadUsers();
      message(ui.usMsg, 'ok', 'Pengguna ' + email + ' ditambahkan dengan role ' + role);
    });
  });

  ui.nmReloadBtn.addEventListener('click', function () { run(ui.nmReloadBtn, ui.nmMsg, loadNaming); });

  ui.nmForm.addEventListener('submit', function (event) {
    event.preventDefault();
    clearMessage(ui.nmMsg);
    let payload;
    try {
      const min = namingNumber('Angka minimum', ui.nmNumMin);
      const max = namingNumber('Angka maksimum', ui.nmNumMax);
      const probability = namingNumber('Peluang angka', ui.nmNumProb);
      if (max < min) {
        throw { error: 'Angka maksimum lebih kecil dari minimum', detail: 'Angka minimum ' + min + ', maksimum ' + max };
      }
      if (probability > 100) {
        throw { error: 'Peluang angka maksimal 100 persen', detail: 'Nilai sekarang ' + probability };
      }
      payload = {
        template: ui.nmTemplate.value.trim(),
        separator: ui.nmSeparator.value,
        first_words: namingJsonArray('first_words', ui.nmFirst.value),
        second_words: namingJsonArray('second_words', ui.nmSecond.value),
        num_min: min,
        num_max: max,
        num_probability: probability,
      };
      if (!payload.template) {
        throw { error: 'Template kosong', detail: 'Isi template, misalnya {first}{sep}{second}{num}' };
      }
    } catch (err) {
      const info = apiFailure(err);
      message(ui.nmMsg, 'error', info.message, info.detail);
      return;
    }
    run(ui.nmSaveBtn, ui.nmMsg, async function () {
      const out = await namingReq({ method: 'PUT', body: JSON.stringify(payload) });
      const rules = (out && out.rules) || payload;
      renderNaming(rules);
      message(ui.nmMsg, 'ok', 'Aturan nama disimpan',
        'template ' + rules.template + ', first_words ' + countOf((rules.first_words || []).length) +
        ', second_words ' + countOf((rules.second_words || []).length) +
        ', angka ' + rules.num_min + '-' + rules.num_max + ' pada peluang ' + rules.num_probability + ' persen');
    });
  });

  ui.dgReloadBtn.addEventListener('click', function () { run(ui.dgReloadBtn, ui.dgMsg, loadDiagnostics); });
}

function boot() {
  if (!API.token) {
    window.location.href = '/app/login.html';
    return;
  }
  bindChrome();
  const user = API.user || {};
  if (user.role !== 'admin') {
    renderGate(user);
    return;
  }
  bindSections();
  ui.imState.hidden = false;
  ui.imState.className = 'state empty';
  ui.imState.replaceChildren(node('span', 'state-text', 'Belum ada impor dijalankan. Tempel isi cloudflare-state.json lalu tekan Impor state.'));
  loadProfiles();
  loadDomains();
  loadTunnel();
  loadCopies();
  bindServer();
  applyHelperGate();
  loadSystemStatus();
  loadUsers();
  loadNaming();
  loadDiagnostics();
}

boot();
