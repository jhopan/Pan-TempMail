// TempMail Frontend — Vanilla JS, no Node.js runtime needed
// Embedded directly in Go binary via go:embed

const API = {
  token: localStorage.getItem('auth_token'),
  user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
  apiKey: window.TEMPMAIL_API_KEY || '',

  async request(path, opts = {}) {
    const headers = { 'Content-Type': 'application/json', ...opts.headers };
    // Prefer JWT if available, fallback to API key
    if (this.token) {
      headers.Authorization = 'Bearer ' + this.token;
    } else if (this.apiKey) {
      headers['X-Email-API-Key'] = this.apiKey;
    }
    const res = await fetch(path, { ...opts, headers });
    if (res.status === 401 && this.token) {
      // JWT expired, clear and retry as guest
      localStorage.removeItem('auth_token');
      localStorage.removeItem('auth_user');
      this.token = null;
      return this.request(path, opts);
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
  // Guest mode: if no JWT, use API key (no redirect to login)
  if (!API.token && !API.apiKey) {
    // No auth at all — redirect to login
    window.location.href = '/app/login.html';
    return;
  }
  // Load saved email or generate random
  const savedUser = localStorage.getItem('email_user');
  if (!savedUser) {
    // Auto-generate random email on first visit
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

function randomEmail() {
  if (cooldown > 0) return;
  const r = Math.random().toString(36).substring(2, 10);
  document.getElementById('username').value = r;
  const domain = document.getElementById('domain').value;
  currentEmail = r + '@' + domain;
  localStorage.setItem('email_user', r);
  updateEmailDisplay();
  selectedMsg = null;
  renderMessage();
  loadInbox();
  reconnectSSE();
  startCooldown(60);
}

function copyEmail() {
  navigator.clipboard.writeText(currentEmail);
  const btn = document.getElementById('copyBtn');
  btn.textContent = '✓ Copied!';
  setTimeout(() => btn.textContent = '📋 Copy', 2000);
}

function startCooldown(seconds) {
  cooldown = seconds;
  const update = () => {
    if (cooldown <= 0) {
      document.querySelectorAll('.cooldown-btn').forEach(b => {
        b.disabled = false;
        b.classList.remove('opacity-50', 'cursor-not-allowed');
      });
      document.getElementById('cooldownNotice')?.remove();
      return;
    }
    document.querySelectorAll('.cooldown-btn').forEach(b => {
      b.disabled = true;
      b.classList.add('opacity-50', 'cursor-not-allowed');
      if (b.id === 'changeBtn') b.textContent = 'Wait ' + cooldown + 's';
      if (b.id === 'randomBtn') b.textContent = '🎲 ' + cooldown + 's';
    });
    cooldown--;
    setTimeout(update, 1000);
  };
  update();
}

async function loadInbox() {
  const res = await API.request('/api/inbox/' + encodeURIComponent(currentEmail));
  if (!res || !res.ok) return;
  const data = await res.json();
  messages = data.data || data || [];
  renderMessageList();
}

function connectSSE() {
  if (sse) sse.close();
  if (!API.token && !API.apiKey) return;
  // Guest mode: use api_key query param; JWT mode: use token query param
  let url = '/sse/' + encodeURIComponent(currentEmail) + '?';
  if (API.token) {
    url += 'token=' + encodeURIComponent(API.token);
  } else {
    url += 'api_key=' + encodeURIComponent(API.apiKey);
  }
  sse = new EventSource(url);
  sse.addEventListener('connected', () => updateSSEStatus('live'));
  sse.addEventListener('new_email', () => { loadInbox(); });
  sse.onerror = () => updateSSEStatus('reconnecting');
}

function reconnectSSE() {
  if (sse) sse.close();
  connectSSE();
}

function updateSSEStatus(status) {
  const dot = document.getElementById('sseDot');
  const text = document.getElementById('sseText');
  if (!dot || !text) return;
  const colors = { live: 'bg-green-500', reconnecting: 'bg-yellow-500 animate-pulse', connecting: 'bg-gray-600' };
  dot.className = 'inline-block w-2 h-2 rounded-full ' + (colors[status] || colors.connecting);
  text.textContent = status === 'live' ? 'Live' : 'Connecting...';
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
  if (!confirm('Delete this message?')) return;
  await API.request('/api/message/' + id, { method: 'DELETE' });
  selectedMsg = null;
  renderMessage();
  loadInbox();
}

async function deleteInbox() {
  if (!confirm('Delete all messages for ' + currentEmail + '?')) return;
  await API.request('/api/inbox/' + encodeURIComponent(currentEmail), { method: 'DELETE' });
  selectedMsg = null;
  messages = [];
  renderMessage();
  renderMessageList();
}

function renderMessageList() {
  const container = document.getElementById('messageList');
  if (!container) return;
  if (messages.length === 0) {
    container.innerHTML = '<div class="p-6 text-center text-gray-600"><div class="text-3xl mb-2">📭</div><div class="text-sm">No messages yet</div><div class="text-xs mt-1">Send an email to ' + currentEmail + '</div></div>';
    return;
  }
  container.innerHTML = messages.map(m => `
    <div onclick="openMessage(${m.id})" class="rounded-lg p-3 cursor-pointer transition-all hover:bg-gray-900 border border-transparent">
      <div class="flex items-center justify-between mb-1">
        <span class="font-semibold text-sm truncate max-w-[200px]">${escapeHtml(m.sender)}</span>
        <span class="text-gray-600 text-xs shrink-0">${formatTime(m.received_at)}</span>
      </div>
      <div class="text-gray-400 text-sm truncate">${escapeHtml(m.subject || '(no subject)')}</div>
    </div>
  `).join('');
}

function renderMessage(msg, attachments = []) {
  const container = document.getElementById('messageView');
  if (!container) return;
  if (!msg) {
    container.innerHTML = '<div class="flex-1 flex items-center justify-center text-gray-600"><div class="text-center"><div class="text-5xl mb-4">📧</div><div class="text-lg">Select an email to read</div></div></div>';
    return;
  }
  // Sanitize HTML: remove scripts and event handlers
  let html = msg.html_body || '';
  html = html.replace(/<script[\s\S]*?<\/script>/gi, '');
  html = html.replace(/on\w+="[^"]*"/gi, '');
  html = html.replace(/on\w+='[^']*'/gi, '');
  html = html.replace(/javascript:/gi, '');

  // Show OTP codes if extracted
  const codes = msg.codes || [];
  const links = msg.links || [];
  let codesHtml = '';
  if (codes.length > 0) {
    codesHtml = '<div class="bg-green-900/20 border border-green-800/50 rounded-lg p-3 mb-3"><div class="text-green-400 text-xs mb-1">🔑 Extracted Codes</div><div class="flex gap-2 flex-wrap">' + codes.map(c => `<span class="bg-green-900/40 text-green-300 px-3 py-1 rounded font-mono text-lg">${escapeHtml(c)}</span>`).join('') + '</div></div>';
  }
  let linksHtml = '';
  if (links.length > 0) {
    linksHtml = '<div class="bg-blue-900/20 border border-blue-800/50 rounded-lg p-3 mb-3"><div class="text-blue-400 text-xs mb-1">🔗 Verification Links</div><div class="flex flex-col gap-1">' + links.map(l => `<a href="${escapeHtml(l)}" target="_blank" class="text-blue-400 hover:text-blue-300 text-sm break-all">${escapeHtml(l)}</a>`).join('') + '</div></div>';
  }

  const attHtml = attachments.length > 0 ? `
    <div class="px-5 py-3 border-b border-gray-800 bg-gray-900/50">
      <div class="text-gray-500 text-xs mb-2">${attachments.length} attachment(s)</div>
      <div class="flex flex-wrap gap-2">
        ${attachments.map(a => `<a href="/api/attachment/${a.id}" download class="flex items-center gap-2 bg-gray-800 hover:bg-gray-700 rounded-lg px-3 py-2 text-sm transition-colors border border-gray-700"><span>📎</span><div><div class="text-gray-200 text-xs font-medium truncate max-w-[150px]">${escapeHtml(a.filename)}</div><div class="text-gray-500 text-xs">${formatSize(a.size)}</div></div></a>`).join('')}
      </div>
    </div>` : '';

  const bodyHtml = html
    ? '<iframe id="msgFrame" sandbox="allow-same-origin" class="w-full border-0 bg-gray-950" style="height:calc(100vh - 200px)"></iframe>'
    : '<div class="p-5"><pre class="whitespace-pre-wrap font-sans text-gray-200 text-sm leading-relaxed">' + escapeHtml(msg.text_body || '') + '</pre></div>';

  container.innerHTML = `
    <div class="p-5 border-b border-gray-800">
      <div class="flex items-start justify-between">
        <div>
          <h2 class="text-xl font-bold mb-1">${escapeHtml(msg.subject || '(no subject)')}</h2>
          <div class="text-gray-400 text-sm">From: ${escapeHtml(msg.sender)}</div>
        </div>
        <button onclick="deleteMessage(${msg.id})" class="bg-red-900/30 hover:bg-red-900/50 text-red-400 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors border border-red-900/50">🗑️ Delete</button>
      </div>
    </div>
    ${attHtml}
    <div class="flex-1 overflow-y-auto">
      ${bodyHtml}
    </div>
  `;
  if (html) {
    const iframe = document.getElementById('msgFrame');
    if (iframe) {
      const doc = iframe.contentDocument;
      const wrapped = '<!DOCTYPE html><html><head><meta charset="utf-8"><style>body{font-family:sans-serif;font-size:14px;line-height:1.6;color:#e2e8f0;background:#0a0a0f;margin:16px;word-wrap:break-word}img{max-width:100%;height:auto}a{color:#a78bfa}table{border-collapse:collapse}td,th{padding:4px}</style></head><body>' + html + '</body></html>';
      doc.open();
      doc.write(wrapped);
      doc.close();
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
