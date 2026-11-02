// Native Node.js fetch; no Commonbeam SDK, wallet, or third-party API endpoint.
const base = process.env.COMMONBEAM_ROUTER_URL ?? "http://127.0.0.1:8080";
let token = process.env.COMMONBEAM_SESSION_TOKEN;
if (!token) {
  const session = await fetch(`${base}/commonbeam/v1/session`, { method: "POST" });
  if (!session.ok) throw new Error(`Session unavailable: HTTP ${session.status}`);
  token = (await session.json()).token;
}

const response = await fetch(`${base}/v1/chat/completions`, {
  method: "POST",
  headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
  body: JSON.stringify({
    model: process.env.COMMONBEAM_MODEL ?? "commonbeam-simulated",
    messages: [{ role: "user", content: "Demonstrate the local test network." }],
    max_tokens: 64,
    stream: true,
  }),
  signal: AbortSignal.timeout(130_000),
});
if (!response.ok) throw new Error(`Chat unavailable: HTTP ${response.status}`);
const reader = response.body.getReader();
const decoder = new TextDecoder();
let buffer = "";
let completed = false;
while (!completed) {
  const { done, value } = await reader.read();
  if (done) break;
  buffer += decoder.decode(value, { stream: true });
  let boundary;
  while ((boundary = buffer.indexOf("\n\n")) >= 0) {
    const block = buffer.slice(0, boundary);
    buffer = buffer.slice(boundary + 2);
    if (!block.startsWith("data: ")) continue;
    const data = block.slice(6);
    if (data === "[DONE]") { completed = true; break; }
    const event = JSON.parse(data);
    if (event.error) throw new Error(`Execution failed: ${event.error.type}`);
    for (const choice of event.choices ?? []) {
      process.stdout.write(choice.delta?.content ?? "");
    }
  }
}
await reader.cancel();
if (!completed) throw new Error("Truncated stream; reconcile job before retrying");
process.stdout.write("\n");
