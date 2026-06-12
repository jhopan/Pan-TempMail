"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/context/AuthContext";

type InboxItem = { id: number; sender: string; subject: string; received_at: string };
type Message = { id: number; sender: string; subject: string; text_body: string; html_body: string };
type Attachment = { id: number; filename: string; content_type: string; size: number };

const DOMAINS = [
  "renunganbot.qzz.io",
  // Tambah domain baru di sini, contoh:
  // "mailku.my.id",
  // "tempmail.co.id",
];

export default function Home() {
  const [username, setUsername] = useState("abc");
  const [domain, setDomain] = useState(DOMAINS[0]);
  const [currentEmail, setCurrentEmail] = useState("");
  const [messages, setMessages] = useState<InboxItem[]>([]);
  const [selected, setSelected] = useState<Message | null>(null);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [loading, setLoading] = useState(false);
  const [authChecked, setAuthChecked] = useState(false);
  const [copied, setCopied] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [sseStatus, setSseStatus] = useState("connecting");
  const sseRef = useRef<EventSource | null>(null);

  const { user, loading: authLoading } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!authLoading && !user) {
      router.push("/login");
    } else if (!authLoading && user) {
      setAuthChecked(true);
    }
  }, [user, authLoading, router]);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setInterval(() => {
      setCooldown((prev) => {
        if (prev <= 1) { clearInterval(timer); return 0; }
        return prev - 1;
      });
    }, 1000);
    return () => clearInterval(timer);
  }, [cooldown > 0]);




  const iframeRef = useRef<HTMLIFrameElement | null>(null);

  useEffect(() => {
    const savedUser = localStorage.getItem("email_user");
    const savedDomain = localStorage.getItem("email_domain");
    if (savedUser) setUsername(savedUser);
    if (savedDomain && DOMAINS.includes(savedDomain)) setDomain(savedDomain);
    const u = savedUser || "abc";
    const d = savedDomain && DOMAINS.includes(savedDomain) ? savedDomain : DOMAINS[0];
    setCurrentEmail(u + "@" + d);
  }, []);

  const loadInbox = useCallback(async () => {
    if (!currentEmail) return;
    try {
      const res = await fetch("/api/inbox/" + encodeURIComponent(currentEmail));
      if (!res.ok) return;
      const data = await res.json();
      setMessages(Array.isArray(data) ? data : []);
    } catch {}
  }, [currentEmail]);

  useEffect(() => {
    if (!currentEmail) return;
    if (sseRef.current) sseRef.current.close();
    setSseStatus("connecting");
    const es = new EventSource("/api/sse/" + encodeURIComponent(currentEmail));
    sseRef.current = es;
    es.addEventListener("connected", () => setSseStatus("live"));
    es.addEventListener("new_email", () => { loadInbox(); });
    es.onerror = () => setSseStatus("reconnecting");
    loadInbox();
    const fallback = setInterval(loadInbox, 30000);
    return () => { es.close(); clearInterval(fallback); };
  }, [currentEmail, loadInbox]);

  function changeEmail() {
    if (cooldown > 0) return;
    const email = username + "@" + domain;
    setCurrentEmail(email); setSelected(null); setAttachments([]);
    localStorage.setItem("email_user", username);
    localStorage.setItem("email_domain", domain);
    setCooldown(60);
  }
  function randomEmail() {
    if (cooldown > 0) return;
    const r = Math.random().toString(36).substring(2, 10);
    setUsername(r);
    const email = r + "@" + domain;
    setCurrentEmail(email); setSelected(null); setAttachments([]);
    localStorage.setItem("email_user", r);
    localStorage.setItem("email_domain", domain);
    setCooldown(60);
  }
  function copyEmail() {
    navigator.clipboard.writeText(currentEmail);
    setCopied(true); setTimeout(() => setCopied(false), 2000);
  }
  async function openMessage(id: number) {
    setLoading(true);
    try {
      const [msgRes, attRes] = await Promise.all([fetch("/api/message/" + id), fetch("/api/message/" + id + "/attachments")]);
      setSelected(await msgRes.json());
      const attData = await attRes.json();
      setAttachments(Array.isArray(attData) ? attData : []);
    } catch { console.error("Failed to load message"); }
    finally { setLoading(false); }
  }
  async function deleteMessage(id: number) {
    if (!confirm("Delete this message?")) return;
    await fetch("/api/message/" + id, { method: "DELETE" });
    setSelected(null); setAttachments([]); loadInbox();
  }
  async function deleteInbox() {
    if (!confirm("Delete all messages for " + currentEmail + "?")) return;
    await fetch("/api/inbox/" + encodeURIComponent(currentEmail), { method: "DELETE" });
    setSelected(null); setAttachments([]); setMessages([]);
  }
  function formatSize(bytes: number) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1048576).toFixed(1) + " MB";
  }
  function fileIcon(ct: string) {
    if (ct && ct.startsWith("image/")) return "\u{1F5BC}\uFE0F";
    if (ct === "application/pdf") return "\u{1F4C4}";
    if (ct && (ct.includes("zip") || ct.includes("archive"))) return "\u{1F4E6}";
    return "\u{1F4CE}";
  }

  // Render HTML email in isolated iframe
  useEffect(() => {
    if (!iframeRef.current || !selected?.html_body) return;
    const iframe = iframeRef.current;
    const doc = iframe.contentDocument;
    if (!doc) return;
    // Sanitize: remove scripts, event handlers
    let html = selected.html_body;
    html = html.replace(/<script[\s\S]*?<\/script>/gi, "");
    html = html.replace(/on\w+="[^"]*"/gi, "");
    html = html.replace(/on\w+='[^']*'/gi, "");
    html = html.replace(/javascript:/gi, "");
    // Wrap in sandboxed HTML with base styles
    const wrapped = '<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;font-size:14px;line-height:1.6;color:#e2e8f0;background:#0a0a0f;margin:16px;word-wrap:break-word}img{max-width:100%;height:auto}a{color:#a78bfa}table{border-collapse:collapse}td,th{padding:4px}</style></head><body>' + html + '</body></html>';
    doc.open();
    doc.write(wrapped);
    doc.close();
    // Auto-resize iframe height
    setTimeout(() => {
      try {
        const h = doc.documentElement.scrollHeight;
        iframe.style.height = Math.max(h + 32, 300) + "px";
      } catch {}
    }, 200);
  }, [selected]);

  if (!authChecked) {
    return (
      <div className="h-screen flex items-center justify-center bg-gray-950">
        <div className="text-purple-400 text-lg animate-pulse">Loading...</div>
      </div>
    );
  }

  return (
    <main className="h-screen flex flex-col bg-gray-950 text-gray-100">
      {/* Top Navigation Bar */}
      <div className="flex items-center justify-between px-5 py-2 border-b border-gray-800 bg-gray-900/50">
        <div className="flex items-center gap-2">
          <span className="text-purple-400 font-bold">⚡</span>
          <span className="text-sm text-gray-400">Logged in as <span className="text-white">{user?.email}</span></span>
        </div>
        <div className="flex items-center gap-3">
          {user?.role === "admin" && (
            <button
              onClick={() => router.push("/admin")}
              className="bg-purple-600/20 hover:bg-purple-600/40 text-purple-400 px-3 py-1 rounded text-sm transition-colors"
            >
              ⚙️ Admin
            </button>
          )}
          <button
            onClick={() => { localStorage.removeItem("auth_token"); localStorage.removeItem("auth_user"); window.location.href = "/login"; }}
            className="bg-red-600/20 hover:bg-red-600/40 text-red-400 px-3 py-1 rounded text-sm transition-colors"
          >
            Logout
          </button>
        </div>
      </div>
      <div className="flex flex-1 overflow-hidden">
      {/* Left panel */}
      <div className="w-80 min-w-80 border-r border-gray-800 flex flex-col">
        <div className="p-5 border-b border-gray-800">
          <h1 className="text-2xl font-bold tracking-tight"><span className="text-purple-400">{"\u26A1"}</span> Temp Mail</h1>
          <div className="flex items-center gap-2 mt-1">
            <p className="text-gray-500 text-sm">Disposable email</p>
            <span className={"inline-block w-2 h-2 rounded-full " + (sseStatus === "live" ? "bg-green-500" : sseStatus === "reconnecting" ? "bg-yellow-500 animate-pulse" : "bg-gray-600")} title={sseStatus} />
          </div>
        </div>

        <div className="p-4 border-b border-gray-800 space-y-3">
          <input value={username} onChange={(e) => setUsername(e.target.value)} disabled={cooldown > 0} placeholder="Enter username" className={"rounded-lg px-3 py-2 w-full text-sm focus:outline-none transition-colors " + (cooldown > 0 ? "bg-gray-800 border border-gray-800 text-gray-500 cursor-not-allowed" : "bg-gray-900 border border-gray-700 focus:border-purple-500")} onKeyDown={(e) => e.key === "Enter" && changeEmail()} />
          <select value={domain} onChange={(e) => setDomain(e.target.value)} disabled={cooldown > 0} className={"rounded-lg px-3 py-2 w-full text-sm focus:outline-none transition-colors appearance-none " + (cooldown > 0 ? "bg-gray-800 border border-gray-800 text-gray-500 cursor-not-allowed" : "bg-gray-900 border border-gray-700 focus:border-purple-500 cursor-pointer")}>
            {DOMAINS.map(d => <option key={d} value={d}>{"@" + d}</option>)}
          </select>
          <div className="flex gap-2">
            <button onClick={changeEmail} disabled={cooldown > 0} className={"rounded-lg px-3 py-2 text-sm font-medium transition-colors flex-1 " + (cooldown > 0 ? "bg-gray-800 text-gray-500 cursor-not-allowed" : "bg-purple-600 hover:bg-purple-700 text-white")}>{cooldown > 0 ? "Wait " + cooldown + "s" : "Change"}</button>
            <button onClick={randomEmail} disabled={cooldown > 0} className={"rounded-lg px-3 py-2 text-sm font-medium transition-colors flex-1 " + (cooldown > 0 ? "bg-gray-800 text-gray-500 cursor-not-allowed" : "bg-gray-700 hover:bg-gray-600 text-white")}>{"\uD83C\uDFB2"} {cooldown > 0 ? cooldown + "s" : "Random"}</button>
          </div>
          {cooldown > 0 && <div className="bg-yellow-900/20 border border-yellow-800/50 rounded-lg p-2 text-yellow-400 text-xs text-center">{"\u23F3"} Cooldown: {cooldown}s — Wait before changing email again</div>}
          <div className="bg-gray-900 rounded-lg p-3">
            <div className="text-gray-500 text-xs mb-1">Your Email</div>
            <div className="font-mono text-sm text-green-400 break-all cursor-pointer hover:text-green-300 transition-colors" onClick={copyEmail} title="Click to copy">{currentEmail}</div>
            {copied && <div className="text-green-400 text-xs mt-1">{"\u2713"} Copied!</div>}
          </div>
          {messages.length > 0 && (
            <button onClick={deleteInbox} className="w-full bg-red-900/30 hover:bg-red-900/50 text-red-400 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors border border-red-900/50">{"\uD83D\uDDD1\uFE0F"} Clear Inbox</button>
          )}
        </div>

        <div className="flex-1 overflow-y-auto">
          {messages.length === 0 ? (
            <div className="p-6 text-center text-gray-600">
              <div className="text-3xl mb-2">{"\uD83D\uDCED"}</div>
              <div className="text-sm">No messages yet</div>
              <div className="text-xs mt-1">Send an email to {currentEmail}</div>
            </div>
          ) : (
            <div className="p-2 space-y-1">
              {messages.map((m) => (
                <div key={m.id} onClick={() => openMessage(m.id)} className={"rounded-lg p-3 cursor-pointer transition-all " + (selected && selected.id === m.id ? "bg-purple-900/40 border border-purple-700" : "hover:bg-gray-900 border border-transparent")}>
                  <div className="flex items-center justify-between mb-1">
                    <span className="font-semibold text-sm truncate max-w-[200px]">{m.sender}</span>
                    <span className="text-gray-600 text-xs shrink-0">{m.received_at ? new Date(m.received_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : ""}</span>
                  </div>
                  <div className="text-gray-400 text-sm truncate">{m.subject}</div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="p-3 border-t border-gray-800 text-center text-xs text-gray-600">
          {messages.length} message{messages.length !== 1 ? "s" : ""} {"\u00B7"} {sseStatus === "live" ? "\uD83D\uDFE2 Live" : "\u23F3 Polling"}
        </div>
      </div>

      {/* Right panel */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {selected ? (
          <>
            <div className="p-5 border-b border-gray-800">
              <div className="flex items-start justify-between">
                <div>
                  <h2 className="text-xl font-bold mb-1">{selected.subject || "(no subject)"}</h2>
                  <div className="text-gray-400 text-sm">From: {selected.sender}</div>
                </div>
                <button onClick={() => deleteMessage(selected.id)} className="bg-red-900/30 hover:bg-red-900/50 text-red-400 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors border border-red-900/50">{"\uD83D\uDDD1\uFE0F"} Delete</button>
              </div>
            </div>
            {attachments.length > 0 && (
              <div className="px-5 py-3 border-b border-gray-800 bg-gray-900/50">
                <div className="text-gray-500 text-xs mb-2">{attachments.length} attachment{attachments.length !== 1 ? "s" : ""}</div>
                <div className="flex flex-wrap gap-2">
                  {attachments.map((att) => (
                    <a key={att.id} href={"/api/attachment/" + att.id} download className="flex items-center gap-2 bg-gray-800 hover:bg-gray-700 rounded-lg px-3 py-2 text-sm transition-colors border border-gray-700">
                      <span>{fileIcon(att.content_type)}</span>
                      <div>
                        <div className="text-gray-200 text-xs font-medium truncate max-w-[150px]">{att.filename}</div>
                        <div className="text-gray-500 text-xs">{formatSize(att.size)}</div>
                      </div>
                    </a>
                  ))}
                </div>
              </div>
            )}
            <div className="flex-1 overflow-y-auto">
              {loading ? (
                <div className="flex items-center justify-center h-full"><div className="text-gray-500 animate-pulse">Loading...</div></div>
              ) : selected.html_body ? (
                <div className="h-full">
                  <iframe ref={iframeRef} sandbox="allow-same-origin" className="w-full min-h-full border-0 bg-gray-950" title="Email content" style={{ height: "calc(100vh - 200px)" }} />
                </div>
              ) : (
                <div className="p-5">
                  <pre className="whitespace-pre-wrap font-sans text-gray-200 text-sm leading-relaxed">{selected.text_body}</pre>
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-gray-600">
            <div className="text-center">
              <div className="text-5xl mb-4">{"\uD83D\uDCE7"}</div>
              <div className="text-lg">Select an email to read</div>
            </div>
          </div>
        )}
      </div>
      </div>
    </main>
  );
}