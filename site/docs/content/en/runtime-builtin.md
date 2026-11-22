---
title: Built-in runtimes
status: published
description: react1, echo1 and conformance.
updated: 2026-11-21
---

## 1. react1

The built-in ReAct Runtime, suited to the usual "model + tool loop".

```sh
levier run "Add 2 and 3." --runtime react1
```

A model must be configured first:

```sh
levier model connect --url ... --key ... --model ...
```

## 2. echo1

A minimal Echo Runtime that calls neither a model nor tools.

```sh
levier run "hello" --runtime echo1
```

Good for verifying:

```text
control plane
credentials
scheduling
events
```

Not suitable for verifying models or tools.

## 3. Built-in CLIs

```text
levier-echo-runtime --config PATH --check --print-config --version --log-level LEVEL
levier-react-runtime --config PATH --check --print-config --version --log-level LEVEL
```

`--print-config` masks the token.

## 4. Choosing

| Goal | Choice |
|---|---|
| Verify the whole chain | `echo1` |
| Regular Agent loop | `react1` |
| Custom prompt / tool ordering | your own `run(ctx)` |
| No model at all | your own Runtime |

## 5. conformance

Echo is the minimal conformance example: it implements only the `run(ctx)`
contract and depends on neither React, LLM nor ToolRouter. Use its behaviour as a
reference when writing your own Runtime.

## Next steps

- Write a Runtime: [Runtime core workflow](runtime-core.md)
- Context: [Context](runtime-context.md)
- Instances and capacity: [Instances and capacity](runtime-remote.md)
