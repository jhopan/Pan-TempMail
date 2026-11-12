// Pan-TempMail frontend, vanilla JS, no Node.js runtime needed
// Embedded directly in Go binary via go:embed

const API = {
  token: localStorage.getItem('auth_token'),
  user: JSON.parse(localStorage.getItem('auth_user') || 'null'),

  async request(path, opts = {}) {
    const headers = { 'Content-Type': 'application/json', ...opts.headers };
    if (this.token) {
      headers.Authorization = 'Bearer ' + this.token;
    }
    const res = await fetch(path, { ...opts, headers });
    if (res.status === 401) {
      window.location.href = '/app/login.html';
      return null;
    }
    return res;
  },

  async login(email, password) {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Login failed');
    }
    const data = await res.json();
    this.token = data.token;
    this.user = data.user;
    localStorage.setItem('auth_token', this.token);
    localStorage.setItem('auth_user', JSON.stringify(this.user));
    return data;
  },

  logout() {
    this.token = null;
    this.user = null;
    localStorage.removeItem('auth_token');
    localStorage.removeItem('auth_user');
    window.location.href = '/app/login.html';
  }
};

// --- Inbox App ---
let currentEmail = '';
let messages = [];
let selectedMsg = null;
let sse = null;
let cooldown = 0;

function getDomains() {
  // Read from DOMAINS env via API
  return window.TEMPMAIL_DOMAINS || ['example.com'];
}

function init() {
  if (!API.token) {
    window.location.href = '/app/login.html';
    return;
  }
  const savedUser = localStorage.getItem('email_user');
  if (!savedUser) {
    randomEmail();
    return;
  }
  const savedDomain = localStorage.getItem('email_domain') || getDomains()[0];
  currentEmail = savedUser + '@' + savedDomain;
  document.getElementById('username').value = savedUser;
  document.getElementById('domain').value = savedDomain;
  updateEmailDisplay();
  loadInbox();
  connectSSE();
}

function updateEmailDisplay() {
  document.getElementById('currentEmail').textContent = currentEmail;
}

function changeEmail() {
  if (cooldown > 0) return;
  const username = document.getElementById('username').value;
  const domain = document.getElementById('domain').value;
  currentEmail = username + '@' + domain;
  localStorage.setItem('email_user', username);
  localStorage.setItem('email_domain', domain);
  updateEmailDisplay();
  selectedMsg = null;
  renderMessage();
  loadInbox();
  reconnectSSE();
  startCooldown(60);
}

async function randomEmail() {
  if (cooldown > 0) return;
  const domain = document.getElementById('domain').value;
  // Call API to generate email using server-side naming rules
  const res = await API.request('/api/inbox', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domain })
  });
  if (res && res.ok) {
    const data = await res.json();
    const parts = data.email.split('@');
    document.getElementById('username').value = parts[0];
    currentEmail = data.email;
    localStorage.setItem('email_user', parts[0]);
    localStorage.setItem('email_domain', parts[1] || domain);
    updateEmailDisplay();
    selectedMsg = null;
    renderMessage();
    loadInbox();
    reconnectSSE();
    startCooldown(60);
  } else {
    // Fallback: client-side random
    const r = Math.random().toString(36).substring(2, 10);
    document.getElementById('username').value = r;
    currentEmail = r + '@' + domain;
    localStorage.setItem('email_user', r);
    updateEmailDisplay();
    selectedMsg = null;
    renderMessage();
    loadInbox();
    reconnectSSE();
    startCooldown(60);
  }
}

function copyEmail() {
  navigator.clipboard.writeText(currentEmail);
  const btn = document.getElementById('copyBtn');
  btn.textContent = '✓ Tersalin';
  setTimeout(() => btn.textContent = '📋 Salin', 2000);
}

function startCooldown(seconds) {
  cooldown = seconds;
  const buttons = () => [document.getElementById('changeBtn'), document.getElementById('randomBtn')].filter(Boolean);
  const update = () => {
    if (cooldown <= 0) {
      buttons().forEach(b => { b.disabled = false; });
      document.getElementById('changeBtn').textContent = 'Ganti';
      document.getElementById('randomBtn').textContent = '🎲 Acak';
      return;
    }
    buttons().forEach(b => { b.disabled = true; });
    document.getElementById('changeBtn').textContent = 'Tunggu ' + cooldown + 's';
    document.getElementById('randomBtn').textContent = '🎲 ' + cooldown + 's';
    cooldown--;
    setTimeout(update, 1000);
  };
  update();
}

function setMsgCount(text) {
  const el = document.getElementById('msgCount');
  if (el) el.textContent = text;
}

async function loadInbox() {
  let data;
  try {
    const res = await API.request('/api/inbox/' + encodeURIComponent(currentEmail));
    if (!res || !res.ok) {
      setMsgCount('tidak terbaca');
      return;
    }
    data = await res.json();
  } catch {
    setMsgCount('tidak terbaca');
    return;
  }
  messages = data.data || data || [];
  const total = typeof data.total === 'number' ? data.total : messages.length;
  setMsgCount(total + ' pesan');
  renderMessageList();
}

function connectSSE() {
  if (sse) sse.close();
  if (!API.token) return;
  updateSSEStatus('connecting');
  sse = new EventSource('/sse/' + encodeURIComponent(currentEmail) + '?token=' + encodeURIComponent(API.token));
  sse.addEventListener('connected', () => updateSSEStatus('live'));
  sse.addEventListener('new_email', () => { loadInbox(); });
  sse.onerror = () => updateSSEStatus('reconnecting');
}

function reconnectSSE() {
  if (sse) sse.close();
  connectSSE();
}

const SSE_DOT_CLASS = { live: 'dot-live', connecting: 'dot-connecting', reconnecting: 'dot-reconnecting' };
const SSE_DOT_TEXT = { live: 'Terhubung', connecting: 'Menghubungkan...', reconnecting: 'Menyambung ulang...' };

function updateSSEStatus(status) {
  const dot = document.getElementById('sseDot');
  const text = document.getElementById('sseText');
  if (!dot || !text) return;
  dot.classList.remove('dot-live', 'dot-connecting', 'dot-reconnecting');
  dot.classList.add(SSE_DOT_CLASS[status] || SSE_DOT_CLASS.connecting);
  text.textContent = SSE_DOT_TEXT[status] || SSE_DOT_TEXT.connecting;
}

async function openMessage(id) {
  const [msgRes, attRes] = await Promise.all([
    API.request('/api/message/' + id),
    API.request('/api/message/' + id + '/attachments'),
  ]);
  if (!msgRes || !msgRes.ok) return;
  selectedMsg = await msgRes.json();
  let attData = [];
  if (attRes?.ok) {
    try { attData = await attRes.json() || []; } catch { attData = []; }
    if (!Array.isArray(attData)) attData = [];
  }
  renderMessage(selectedMsg, attData);
}

async function deleteMessage(id) {
  if (!confirm('Hapus pesan ini?')) return;
  await API.request('/api/message/' + id, { method: 'DELETE' });
  selectedMsg = null;
  renderMessage();
  loadInbox();
}

async function deleteInbox() {
  if (!confirm('Hapus semua pesan untuk ' + currentEmail + '?')) return;
  await API.request('/api/inbox/' + encodeURIComponent(currentEmail), { method: 'DELETE' });
  selectedMsg = null;
  messages = [];
  setMsgCount('0 pesan');
  renderMessage();
  renderMessageList();
}

function renderMessageList() {
  const container = document.getElementById('messageList');
  if (!container) return;
  if (messages.length === 0) {
    container.innerHTML = '<div class="empty"><div class="ico">📭</div><div class="txt">Belum ada pesan</div><div class="sub">Kirim email ke ' + currentEmail + ' untuk melihatnya di sini</div></div>';
    return;
  }
  container.innerHTML = messages.map(m => `
    <div onclick="openMessage(${m.id})" class="msg-item">
      <span class="time">${formatTime(m.received_at)}</span>
      <div class="from">${escapeHtml(m.sender)}</div>
      <div class="subj">${escapeHtml(m.subject || '(tanpa subjek)')}</div>
    </div>
  `).join('');
}

function renderMessage(msg, attachments = []) {
  const container = document.getElementById('messageView');
  if (!container) return;
  if (!msg) {
    container.innerHTML = '<div class="placeholder"><div><div class="ico">📧</div><div class="txt">Pilih pesan untuk dibaca</div></div></div>';
    return;
  }
  let html = msg.html_body || '';
  html = html.replace(/<script[\s\S]*?<\/script>/gi, '');
  html = html.replace(/on\w+="[^"]*"/gi, '');
  html = html.replace(/on\w+='[^']*'/gi, '');
  html = html.replace(/javascript:/gi, '');

  const attHtml = attachments.length > 0 ? `
    <div class="att-area">
      <div class="lbl">${attachments.length} lampiran</div>
      <div class="att-list">
        ${attachments.map(a => `<a href="/api/attachment/${a.id}" download class="att-item"><span>📎</span>${escapeHtml(a.filename)} (${formatSize(a.size)})</a>`).join('')}
      </div>
    </div>` : '';

  const bodyHtml = html
    ? '<iframe id="msgFrame" sandbox="allow-same-origin" srcdoc=""></iframe>'
    : '<pre>' + escapeHtml(msg.text_body || '') + '</pre>';

  container.innerHTML = `
    <div class="msg-head">
      <button onclick="deleteMessage(${msg.id})" class="del-btn">🗑️ Hapus</button>
      <h2>${escapeHtml(msg.subject || '(tanpa subjek)')}</h2>
      <div class="from">Dari: ${escapeHtml(msg.sender)}</div>
    </div>
    ${attHtml}
    <div class="msg-body">${bodyHtml}</div>
  `;
  if (html) {
    const iframe = document.getElementById('msgFrame');
    if (iframe) {
      const wrapped = '<!DOCTYPE html><html><head><meta charset="utf-8"><style>body{font-family:sans-serif;font-size:14px;line-height:1.6;color:#e2e8f0;background:#0a0a0f;margin:16px;word-wrap:break-word}img{max-width:100%;height:auto}a{color:#8b5cf6}table{border-collapse:collapse}td,th{padding:4px}</style></head><body>' + html + '</body></html>';
      iframe.srcdoc = wrapped;
    }
  }
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
function formatTime(ts) {
  if (!ts) return '';
  return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1048576).toFixed(1) + ' MB';
}
