# Levier LangGraph Runtime

Optional LangGraph driver for Levier. This package is intentionally separate
from `levier-runtime`; installing the core Runtime SDK never installs
LangGraph.

```python
from langgraph.graph import StateGraph, START, END
from levier_langgraph_runtime import LangGraphDriver
from levier_runtime import Runtime


def build_graph(checkpointer):
    graph = StateGraph(GraphState)
    graph.add_node("work", work)
    graph.add_edge(START, "work")
    graph.add_edge("work", END)
    return graph.compile(checkpointer=checkpointer)


Runtime.start_driver(LangGraphDriver(build_graph))
```

The driver maps LangGraph node lifecycle to custom events, persists the
LangGraph checkpointer state through `ctx.checkpoint`, and returns `waiting`
when the graph pauses at an interrupt. Resume uses the standard Levier Run
resume entry point; the driver restores the LangGraph checkpointer state and
continues with `Command(resume=...)`.

When the resume request carries `{"resume": ...}`, the value is delivered as
`ctx.resume_value` with `ctx.resume_value_set=True`. The driver prefers that
value over its configured fallback.

This is an I2 prototype. It deliberately does not add LangGraph to the core
Runtime SDK, and it does not implement a separate graph execution engine.
