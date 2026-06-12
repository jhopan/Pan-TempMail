import { NextRequest, NextResponse } from "next/server";

async function proxy(req: NextRequest, method: string, path: string = "") {
  const auth = req.headers.get("authorization") || "";
  const url = `http://127.0.0.1:8080/api/admin/users${path}`;
  const opts: RequestInit = { method, headers: { "Authorization": auth, "Content-Type": "application/json" } };
  
  if (method !== "GET" && method !== "DELETE") {
    const body = await req.text();
    opts.body = body;
  }
  
  const res = await fetch(url, opts);
  const text = await res.text();
  try {
    const data = JSON.parse(text);
    return NextResponse.json(data, { status: res.status });
  } catch {
    return new NextResponse(text, { status: res.status });
  }
}

export async function GET(req: NextRequest) {
  return proxy(req, "GET");
}

export async function POST(req: NextRequest) {
  return proxy(req, "POST");
}

export async function PUT(req: NextRequest) {
  const url = new URL(req.url);
  const parts = url.pathname.split("/");
  const userId = parts[parts.length - 1];
  const isPassword = url.pathname.includes("/password");
  const path = isPassword ? `/${userId}/password` : `/${userId}`;
  return proxy(req, "PUT", path);
}

export async function DELETE(req: NextRequest) {
  const url = new URL(req.url);
  const parts = url.pathname.split("/");
  const userId = parts[parts.length - 1];
  return proxy(req, "DELETE", `/${userId}`);
}
