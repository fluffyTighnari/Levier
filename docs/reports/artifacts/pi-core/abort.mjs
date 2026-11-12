import { Agent } from "@earendil-works/pi-agent-core";
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai";

const usage = () => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } });
const model = { id: "fake", name: "fake", api: "openai-completions", provider: "fake", baseUrl: "http://127.0.0.1:1", input: ["text"], reasoning: false, contextWindow: 1024, maxTokens: 128, cost: usage().cost };
const msg = (content, stopReason) => ({ role: "assistant", content, api: model.api, provider: model.provider, model: model.id, usage: usage(), stopReason, timestamp: Date.now() });

const events = [];
const agent = new Agent({
  initialState: { systemPrompt: "t", model },
  streamFn: async (_model, _ctx, options) => {
    const stream = createAssistantMessageEventStream();
    const partial = msg([], "pending");
    stream.push({ type: "start", partial });
    const timer = setTimeout(() => {
      const final = msg([{ type: "text", text: "late" }], "stop");
      stream.push({ type: "done", reason: "stop", message: final });
    }, 1000);
    options?.signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      stream.push({ type: "error", reason: "aborted", error: msg([], "aborted") });
    });
    return stream;
  },
});
agent.subscribe((event) => events.push(event.type === "agent_end" ? `agent_end:${event.messages.at(-1)?.stopReason}` : event.type));
const started = performance.now();
const run = agent.prompt("wait");
setTimeout(() => agent.abort(), 50);
await run;
console.log(JSON.stringify({ abort_ms: Math.round(performance.now() - started), events }, null, 2));
