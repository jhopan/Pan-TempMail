import { NextRequest, NextResponse } from "next/server";

function getAuthHeader(req: NextRequest): HeadersInit {
  const auth = req.headers.get("authorization") || "";
  return auth ? { Authorization: auth } : {};
}

export async function GET(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  try {
    const res = await fetch(`http://127.0.0.1:8080/message/${id}`, {
      headers: getAuthHeader(req),
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}

export async function DELETE(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  try {
    const res = await fetch(`http://127.0.0.1:8080/message/${id}`, {
      method: "DELETE",
      headers: getAuthHeader(req),
    });
    const data = await res.json();
    return NextResponse.json(data, { status: res.status });
  } catch {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}
