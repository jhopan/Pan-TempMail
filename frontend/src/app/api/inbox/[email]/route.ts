import { NextRequest, NextResponse } from "next/server";

export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ email: string }> }
) {
  const { email } = await params;
  const decodedEmail = decodeURIComponent(email);

  try {
    const res = await fetch(`http://127.0.0.1:8080/inbox/${decodedEmail}`);
    if (!res.ok) {
      return NextResponse.json({ error: "Backend error" }, { status: res.status });
    }
    const data = await res.json();
    return NextResponse.json(data);
  } catch {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}

export async function DELETE(
  _req: NextRequest,
  { params }: { params: Promise<{ email: string }> }
) {
  const { email } = await params;
  const decodedEmail = decodeURIComponent(email);

  try {
    const res = await fetch(`http://127.0.0.1:8080/inbox/${decodedEmail}`, { method: "DELETE" });
    if (!res.ok) {
      return NextResponse.json({ error: "Backend error" }, { status: res.status });
    }
    return NextResponse.json({ success: true });
  } catch {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}
