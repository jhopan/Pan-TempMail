import { NextRequest } from "next/server";

export async function GET(
  req: NextRequest,
  { params }: { params: Promise<{ email: string }> }
) {
  const { email } = await params;
  const decodedEmail = decodeURIComponent(email);
  const token = req.nextUrl.searchParams.get("token") || "";

  const url = `http://127.0.0.1:8080/sse/${encodeURIComponent(decodedEmail)}?token=${encodeURIComponent(token)}`;

  const backendRes = await fetch(url, {
    headers: { Accept: "text/event-stream" },
  });

  const stream = new ReadableStream({
    start(controller) {
      const reader = backendRes.body!.getReader();
      const decoder = new TextDecoder();

      function push() {
        reader.read().then(({ done, value }) => {
          if (done) {
            controller.close();
            return;
          }
          controller.enqueue(value);
          push();
        });
      }
      push();
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
}
