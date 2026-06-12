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
    const res = await fetch(`http://127.0.0.1:8080/message/${id}/attachments`, {
      headers: getAuthHeader(req),
    });
    if (!res.ok) return NextResponse.json([], { status: 200 });
    const data = await res.json();
    return NextResponse.json(data);
  } catch {
    return NextResponse.json([], { status: 200 });
  }
}
