import { NextRequest, NextResponse } from "next/server";

export async function GET(req: NextRequest) {
  try {
    const auth = req.headers.get("authorization") || "";
    const res = await fetch("http://127.0.0.1:8080/api/auth/me", {
      headers: { "Authorization": auth },
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch (e) {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}
