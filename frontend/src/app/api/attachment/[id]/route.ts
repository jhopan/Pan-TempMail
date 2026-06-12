import { NextRequest, NextResponse } from "next/server";

export async function GET(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params;
  const auth = req.headers.get("authorization") || "";
  const token = req.nextUrl.searchParams.get("token") || "";
  
  const headers: HeadersInit = {};
  if (auth) headers["Authorization"] = auth;
  else if (token) headers["Authorization"] = "Bearer " + token;

  try {
    const res = await fetch(`http://127.0.0.1:8080/attachment/${id}`, { headers });
    if (!res.ok) {
      return NextResponse.json({ error: "Not found" }, { status: res.status });
    }
    const buffer = await res.arrayBuffer();
    const contentType = res.headers.get("content-type") || "application/octet-stream";
    const disposition = res.headers.get("content-disposition") || `attachment; filename="${id}"`;
    
    return new NextResponse(buffer, {
      headers: {
        "Content-Type": contentType,
        "Content-Disposition": disposition,
      },
    });
  } catch {
    return NextResponse.json({ error: "Backend unreachable" }, { status: 502 });
  }
}
