import { Agent } from "@earendil-works/pi-agent-core";
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai";

const usage = () => ({
  input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
});

const model = {
  id: "fake-model", name: "fake", api: "openai-completions", provider: "fake",
  baseUrl: "http://127.0.0.1:1", input: ["text"], reasoning: false,
  contextWindow: 8192, maxTokens: 1024, cost: usage().cost,
};

const message = (content, stopReason) => ({
  role: "assistant", content, api: model.api, provider: model.provider,
  model: model.id, usage: usage(), stopReason, timestamp: Date.now(),
});

function toolCallStream() {
  const stream = createAssistantMessageEventStream();
  const partial = message([], "pending");
  const call = { type: "toolCall", id: "call-1", name: "add", arguments: { a: 2, b: 3 } };
  const final = message([call], "toolUse");
  queueMicrotask(() => {
    stream.push({ type: "start", partial });
    stream.push({ type: "toolcall_end", contentIndex: 0, toolCall: call, partial: final });
    stream.push({ type: "done", reason: "toolUse", message: final });
  });
  return stream;
}

function textStream(text) {
  const stream = createAssistantMessageEventStream();
  const partial = message([], "pending");
  const final = message([{ type: "text", text }], "stop");
  queueMicrotask(() => {
    stream.push({ type: "start", partial });
    stream.push({ type: "text_delta", contentIndex: 0, delta: text, partial: final });
    stream.push({ type: "text_end", contentIndex: 0, content: text, partial: final });
    stream.push({ type: "done", reason: "stop", message: final });
  });
  return stream;
}

let calls = 0;
const events = [];
const toolResults = [];
const agent = new Agent({
  initialState: {
    systemPrompt: "test",
    model,
    tools: [{
      name: "add", label: "Add", description: "add two numbers",
      parameters: {
        type: "object",
        properties: { a: { type: "number" }, b: { type: "number" } },
        required: ["a", "b"],
      },
      execute: async (id, params) => {
        toolResults.push({ id, params });
        return { content: [{ type: "text", text: String(params.a + params.b) }], details: {} };
      },
    }],
  },
  streamFn: async () => (++calls === 1 ? toolCallStream() : textStream("5")),
});

agent.subscribe((event) => {
  events.push(event.type === "message_update" ? `message_update:${event.assistantMessageEvent.type}` : event.type);
});

const started = performance.now();
await agent.prompt("add 2 and 3");
const elapsed = performance.now() - started;

console.log(JSON.stringify({
  prompt_ms: Math.round(elapsed),
  stream_calls: calls,
  tool_results: toolResults,
  event_counts: events.reduce((acc, name) => (acc[name] = (acc[name] || 0) + 1, acc), {}),
  final_messages: agent.state.messages.map((m) => ({ role: m.role, content: typeof m.content === "string" ? m.content : m.content.length })),
  state_fields: Object.keys(agent.state),
}, null, 2));
